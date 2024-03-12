package metaservice

import (
	"github.com/dataswap/go-metadata/utils"

	mt "github.com/txaty/go-merkletree"
)

// ChallengeProofs represents the challenge proofs data structure.
type ChallengeProofs struct {
	RandomSeed uint64
	Leaves     []string
	Siblings   [][]string
	Paths      []uint32
}

// NewChallengeProofs creates a new ChallengeProofs instance from the provided randomness and proof map.
func NewChallengeProofs(randomness uint64, proof map[string]mt.Proof) *ChallengeProofs {
	var challengeProofs ChallengeProofs

	challengeProofs.RandomSeed = randomness

	for key, value := range proof {
		challengeProofs.Leaves = append(challengeProofs.Leaves, key)

		siblings := make([]string, len(value.Siblings))
		for i, sibling := range value.Siblings {
			siblings[i] = utils.ConvertToHexPrefix(sibling)
		}
		challengeProofs.Siblings = append(challengeProofs.Siblings, siblings)

		challengeProofs.Paths = append(challengeProofs.Paths, value.Path)
	}
	return &challengeProofs
}

// NewChallengeProofsFromFile creates a new ChallengeProofs instance by reading from the provided file path.
func NewChallengeProofsFromFile(filePath string) (*ChallengeProofs, error) {

	var challengeProofs ChallengeProofs
	err := utils.ReadJson(filePath, &challengeProofs)
	if err != nil {
		return nil, err
	}

	return &challengeProofs, nil
}

// save saves the ChallengeProofs instance to the provided file path.
func (c *ChallengeProofs) save(filePath string) error {

	return utils.WriteJson(filePath, "\t", c)
}

// proof returns a map of proof data for the ChallengeProofs instance.
func (c *ChallengeProofs) proof() map[string]mt.Proof {
	proofMap := make(map[string]mt.Proof)

	for i, leaf := range c.Leaves {

		siblings := make([][]byte, len(c.Siblings[i]))
		for j, sibling := range c.Siblings[i] {
			siblings[j], _ = utils.ParseHexWithPrefix(sibling)
		}

		proofMap[leaf] = mt.Proof{
			Siblings: siblings,
			Path:     c.Paths[i],
		}
	}

	return proofMap
}
