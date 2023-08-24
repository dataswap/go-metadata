# go-metadata

Implement mapping information collection between source data and target car files during the Dag construction process

## Features

go-metadata is a publicly available library that includes functions for source data sampling, CAR generation, dataset proofs, dataset proof challenges, and validation tools.

## Documentation

```shell
To be added
```

## Development

### dependencies

#### go

To build go-metadata, you need a working installation of Go 1.20.1 or higher:

```shell
wget -c https://golang.org/dl/go1.20.1.linux-amd64.tar.gz -O - | sudo tar -xz -C /usr/local
```

**TIP:**
You'll need to add `/usr/local/go/bin` to your path. For most Linux distributions you can run something like:

```shell
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.bashrc && source ~/.bashrc
```

See the [official Golang installation instructions](https://golang.org/doc/install) if you get stuck.

### Clone the repository

```shell
git clone https://github.com/dataswap/go-metadata.git
cd go-metadata/
```

### Build

```shell
go build -v ./cmd
```

### Test

```shell
go test -v ./service
```

## Installation

```shell
To be added
```

## Usage

* Data set original file scanning, car file generation, Mapping File Generation
* DatasetProof
  * The DP needs to submit the DatasetProof to the Dataswap contract
  * DA compute Merkle-Tree for challenge proof
* DatasetVerification
  * DA uses data proof verification tools to generate dataset challenge proof verification information.
* Other tools
  * compute commp CID(PieceCID)
  * dump commp info

```shell
$ meta 
NAME:
   meta - Utility for working with car files

USAGE:
   meta [global options] command [command options] [arguments...]

COMMANDS:
   create       Create a car file
   list, l, ls  List the CIDs in a car
   proof        compute proof of merkle-tree
   verify       verify challenge proofs of merkle-tree
   tools        
   help, h      Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

### Create car file

```shell
$ meta create -h
NAME:
   meta create - Create a car file

USAGE:
   meta create command [command options] [arguments...]

COMMANDS:
   car      Create a car file
   chunks   Create car chunks
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### DatasetProof

* The DA challenges specific DatasetLeafHashes (CarRootHashes) and CarLeafHashes through random challenges.
* The DP needs to submit the DatasetProof to the business contract, where the DatasetMerkleTree is stored on-chain, and the CarProofs are stored on the Filecoin network (to save on-chain resources).

```shell
$ meta proof -h
NAME:
   meta proof - compute proof of merkle-tree

USAGE:
   meta proof command [command options] [arguments...]

COMMANDS:
   chanllenge-proof  compute proof of merkle-tree
   dataset-proof     compute dataset proof of commPs
   help, h           Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

### DatasetVerification

* The DA submits the challenged DatasetHash Merkle Proof and CarRootHash Merkle Proof to the blockchain as challenge proof information for verification.

```shell
$ meta verify -h
NAME:
   meta verify - verify challenge proofs of merkle-tree

USAGE:
   meta verify [command options] <randomness> <cachePath>

OPTIONS:
   --help, -h  show help
```

## Contribute

go-metadata is a universally open project and welcomes contributions of all kinds: code, docs, and more. However, before making a contribution, we ask you to heed these recommendations:

* If the change is complex and requires prior discussion, [open an issue](https://github.com/dataswap/go-metadata/issues) or a [discussion](https://github.com/dataswap/specs) to request feedback before you start working on a pull request. This is to avoid disappointment and sunk costs, in case the change is not actually needed or accepted.
* Please refrain from submitting [PRs](https://github.com/dataswap/go-metadata/pulls) to adapt existing code to subjective preferences. The changeset should contain functional or technical improvements/enhancements, bug fixes, new features, or some other clear material contribution. Simple stylistic changes are likely to be rejected in order to reduce code churn.

When implementing a change:

* Adhere to the standard Go formatting guidelines, e.g. [Effective Go](https://golang.org/doc/effective_go.html). Run `go fmt`.
* Stick to the idioms and patterns used in the codebase. Familiar-looking code has a higher chance of being accepted than eerie code. Pay attention to commonly used variable and parameter names, avoidance of naked returns, error handling patterns, etc.
* Comments: follow the advice on the [Commentary](https://golang.org/doc/effective_go.html#commentary) section of Effective Go.
* Minimize code churn. Modify only what is strictly necessary. Well-encapsulated changesets will get a quicker response from maintainers.
* Lint your code with [`golangci-lint`](https://golangci-lint.run) (CI will reject your PR if unlinted).
* Add tests.
* Title the PR in a meaningful way and describe the rationale and the thought process in the PR description.
* Write clean, thoughtful, and detailed [commit messages](https://chris.beams.io/posts/git-commit/). This is even more important than the PR description, because commit messages are stored _inside_ the Git history. One good rule is: if you are happy posting the commit message as the PR description, then it's a good commit message.

## License

This project is licensed under [GPL-3.0-or-later](https://www.gnu.org/licenses/gpl-3.0.en.html).
