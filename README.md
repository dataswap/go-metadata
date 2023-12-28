<h1 align="center">Welcome to @dataswap/go-metadata 👋</h1>
<p>
  <img alt="Version" src="https://img.shields.io/badge/version-0.0.0-blue.svg?cacheSeconds=2592000" />
  <a href="https://github.com/dataswap/go-metadata#readme" target="_blank">
    <img alt="Documentation" src="https://img.shields.io/badge/documentation-yes-brightgreen.svg" />
  </a>
  <a href="https://github.com/dataswap/go-metadata/blob/main/LICENSE" target="_blank">
    <img alt="License: MIT and APACHE" src="https://img.shields.io/badge/License-MIT and APACHE-yellow.svg" />
  </a>
</p>

### 🏠 [Homepage](https://github.com/dataswap/go-metadata)
# go-metadata

Implement mapping information collection between source data and target car files during the Dag construction process

## Features

go-metadata is a publicly available library that includes functions for source data sampling, CAR generation, dataset proofs, dataset proof challenges, and validation tools.

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

## Author

👤 **dataswap**

* GitHub: [@dataswap](https://github.com/dataswap)

## 🤝 Contributing

Contributions, issues and feature requests are welcome!<br />Feel free to check [issues page](https://github.com/dataswap/go-metadata/issues). You can also take a look at the [contributing guide](https://github.com/dataswap/go-metadata/blob/main/CONTRIBUTING.md).

## Show your support

Give a ⭐️ if this project helped you!

## 📝 License

Copyright © 2023 [dataswap](https://github.com/dataswap).<br />
This project is [MIT and APACHE](https://github.com/dataswap/go-metadata/blob/main/LICENSE) licensed.
