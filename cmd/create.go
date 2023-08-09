package main

import (
	"context"
	"io"
	"os"

	"github.com/dataswap/go-metadata/libs"
	metaservice "github.com/dataswap/go-metadata/service"
	"github.com/filecoin-project/boost-gfm/stores"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	"github.com/ipfs/go-cidutil/cidenc"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	"github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var MaxTraversalLinks uint64 = 32 * (1 << 20)

var createCmd = &cli.Command{
	Name:      "create",
	Usage:     "Create a car file",
	ArgsUsage: "<inputPath> <outputPath>",
	Action:    CreateCar,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "json",
			Usage: "The meta file to write to",
		},
	},
}

func CreateCar(cctx *cli.Context) error {
	if cctx.Args().Len() != 2 {
		return xerrors.Errorf("usage: generate-car <inputPath> <outputPath>")
	}

	inPath := cctx.Args().First()
	outPath := cctx.Args().Get(1)

	ftmp, err := os.CreateTemp("", "")
	if err != nil {
		return xerrors.Errorf("failed to create temp file: %w", err)
	}
	_ = ftmp.Close() // close; we only want the path.

	tmp := ftmp.Name()
	defer os.Remove(tmp) //nolint:errcheck
	msrv := metaservice.New()
	// generate and import the UnixFS DAG into a filestore (positional reference) CAR.
	root, err := CreateFilestore(cctx.Context, inPath, tmp, msrv)
	if err != nil {
		return xerrors.Errorf("failed to import file using unixfs: %w", err)
	}
	msrv.SetCarRoot(root)

	// open the positional reference CAR as a filestore.
	fs, err := stores.ReadOnlyFilestore(tmp)
	if err != nil {
		return xerrors.Errorf("failed to open filestore from carv2 in path %s: %w", outPath, err)
	}
	defer fs.Close() //nolint:errcheck

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}

	// build a dense deterministic CAR (dense = containing filled leaves)
	if err := car.NewSelectiveCar(
		cctx.Context,
		fs,
		[]car.Dag{{
			Root:     root,
			Selector: selectorparse.CommonSelector_ExploreAllRecursively,
		}},
		car.MaxTraversalLinks(MaxTraversalLinks),
	).Write(
		msrv.GenCarWriter(f, outPath, true),
	); err != nil {
		return xerrors.Errorf("failed to write CAR to output file: %w", err)
	}

	err = f.Close()
	if err != nil {
		return err
	}

	encoder := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}

	log.Info("Payload CID: ", encoder.Encode(root))

	return msrv.PrintJson(cctx.String("json"))
}

func CreateFilestore(ctx context.Context, srcPath string, dstPath string, msrv *metaservice.MetaService) (cid.Cid, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open input file: %w", err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to stat file :%w", err)
	}

	file, err := files.NewReaderPathFile(srcPath, src, stat)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create reader path file: %w", err)
	}

	f, err := os.CreateTemp("", "")
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temp file: %w", err)
	}
	_ = f.Close() // close; we only want the path.

	tmp := f.Name()
	defer os.Remove(tmp) //nolint:errcheck

	// Step 1. Compute the UnixFS DAG and write it to a CARv2 file to get
	// the root CID of the DAG.
	fstore, err := stores.ReadWriteFilestore(tmp)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temporary filestore: %w", err)
	}

	finalRoot1, err := Build(ctx, file, fstore, true, srcPath, nil)
	if err != nil {
		_ = fstore.Close()
		return cid.Undef, xerrors.Errorf("failed to import file to store to compute root: %w", err)
	}

	if err := fstore.Close(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car filestore: %w", err)
	}

	// Step 2. We now have the root of the UnixFS DAG, and we can write the
	// final CAR for real under `dst`.
	bs, err := stores.ReadWriteFilestore(dstPath, finalRoot1)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create a carv2 read/write filestore: %w", err)
	}

	// rewind file to the beginning.
	if _, err := src.Seek(0, 0); err != nil {
		return cid.Undef, xerrors.Errorf("failed to rewind file: %w", err)
	}

	finalRoot2, err := Build(ctx, file, bs, true, srcPath, msrv)
	if err != nil {
		_ = bs.Close()
		return cid.Undef, xerrors.Errorf("failed to create UnixFS DAG with carv2 blockstore: %w", err)
	}

	if err := bs.Close(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car blockstore: %w", err)
	}

	if finalRoot1 != finalRoot2 {
		return cid.Undef, xerrors.New("roots do not match")
	}

	return finalRoot2, nil
}

const UnixfsLinksPerLevel = 1024

func Build(ctx context.Context, reader io.Reader, into bstore.Blockstore, filestore bool, srcPath string, msrv *metaservice.MetaService) (cid.Cid, error) {
	b, err := CidBuilder()
	if err != nil {
		return cid.Undef, err
	}

	bsvc := blockservice.New(into, offline.Exchange(into))
	dags := merkledag.NewDAGService(bsvc)
	bufdag := ipld.NewBufferedDAG(ctx, dags)
	var spl chunker.Splitter
	//var db helpers.Helper
	params := helpers.DagBuilderParams{
		Maxlinks:   UnixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: b,
		Dagserv:    bufdag,
		NoCopy:     filestore,
	}

	if msrv != nil {
		spl = msrv.GenSplitter(reader, srcPath, true)
		params.Dagserv = msrv.GenDagService(bufdag)
	} else {
		spl = chunker.NewSizeSplitter(reader, int64(libs.UnixfsChunkSize))
	}

	db, err := params.New(spl)
	if err != nil {
		return cid.Undef, err
	}

	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	if err := bufdag.Commit(); err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}

var DefaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

func CidBuilder() (cid.Builder, error) {
	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return nil, xerrors.Errorf("failed to initialize UnixFS CID Builder: %w", err)
	}
	prefix.MhType = DefaultHashFunction
	b := cidutil.InlineBuilder{
		Builder: prefix,
		Limit:   126,
	}
	return b, nil
}
