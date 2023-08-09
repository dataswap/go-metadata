package metaservice

import ipld "github.com/ipfs/go-ipld-format"

type Options struct {
	dagService ipld.DAGService
}

type Option func(o *Options)

func newOptions(opts ...Option) *Options {
	options := Options{}

	for _, o := range opts {
		o(&options)
	}

	return &options
}

func DagService(ds ipld.DAGService) Option {
	return func(o *Options) {
		o.dagService = ds
	}
}
