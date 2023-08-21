package metaservice

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/dataswap/go-metadata/types"
	"github.com/ipfs/go-cid"
	"gotest.tools/assert"
)

func TestMappingService_InsertMapping(t *testing.T) {
	// Initialize a MappingService instance.
	ms := New( /* options */ )
	c, err := cid.Decode("bafybeictelraxs64igqaz7drzanu2rdt4kk5veo7jp2gea5g3i3evjkrdq")
	if err != nil {
		t.Errorf("decode cid failed")
	}

	// Create a mock ChunkMapping instance.
	mockMapping := &types.ChunkMapping{
		SrcPath:   "input/test1.json",
		SrcOffset: 0,
		Size:      93,
		DstOffset: 14987673,
		ChunkSize: 139,
		NodeType:  2,
		Cid:       c,
	}

	// Insert the mapping in the InsertMapping method.
	err = ms.insertMapping(mockMapping.Cid, mockMapping, 100)
	if err != nil {
		t.Errorf("Error inserting mapping: %v", err)
	}

	// Insert the same mapping again, should return an error.
	err = ms.insertMapping(mockMapping.Cid, mockMapping, 200)
	if err == nil {
		t.Errorf("Expected error when inserting duplicate mapping, but got none")
	}
}

func TestMappingService_GetAllChunkMappings(t *testing.T) {
	// Initialize a MappingService instance.
	ms := New( /* options */ )

	c1, err := cid.Decode("bafybeictelraxs64igqaz7drzanu2rdt4kk5veo7jp2gea5g3i3evjkrdq")
	if err != nil {
		t.Errorf("decode cid failed")
	}

	// Create a mock ChunkMapping instance.
	mockMapping1 := &types.ChunkMapping{
		SrcPath:   "input/test1.json",
		SrcOffset: 0,
		Size:      93,
		DstOffset: 14987673,
		ChunkSize: 139,
		NodeType:  2,
		Cid:       c1,
	}

	// Insert the mock mappings into the mappings map for testing.
	if err := ms.insertMapping(c1, mockMapping1, 93); err != nil {
		t.Errorf("Error insert mapping: %v", err)
	}

	c2, err := cid.Decode("bafybeidh5ub7o3v452t5r66inkntzpyvzzlv77vhxdr52tei26qxka6vke")
	if err != nil {
		t.Errorf("decode cid failed")
	}

	mockMapping2 := &types.ChunkMapping{
		SrcPath:   "input/test2.json",
		SrcOffset: 0,
		Size:      90,
		DstOffset: 14987812,
		ChunkSize: 136,
		NodeType:  2,
		Cid:       c2,
	}

	// Insert the mock mappings into the mappings map for testing.
	if err := ms.insertMapping(c2, mockMapping2, 90); err != nil {
		t.Errorf("Error insert mapping: %v", err)
	}

	// Call the GetAllChunkMappings function.
	mappings, err := ms.GetAllChunkMappings()
	if err != nil {
		t.Errorf("Error getting chunk mappings: %v", err)
	}

	// Perform assertions to verify the returned mappings.
	if len(mappings) != 2 {
		t.Errorf("Expected 2 mappings, but got %d", len(mappings))
	}
}

func TestMappingService_UpdateMapping(t *testing.T) {
	// Initialize a MappingService instance.
	ms := New( /* options */ )

	c1, err := cid.Decode("bafybeictelraxs64igqaz7drzanu2rdt4kk5veo7jp2gea5g3i3evjkrdq")
	if err != nil {
		t.Errorf("decode cid failed")
	}

	// Create a mock ChunkMapping instance.
	mockMapping1 := &types.ChunkMapping{
		SrcPath:   "input/test1.json",
		SrcOffset: 0,
		Size:      93,
		DstOffset: 14987673,
		ChunkSize: 139,
		NodeType:  2,
		Cid:       c1,
	}

	// Insert the mock mappings into the mappings map for testing.
	if err := ms.insertMapping(c1, mockMapping1, 93); err != nil {
		t.Errorf("Error insert mapping: %v", err)
	}
	var offset uint64 = 123
	// Call the updateMapping function.
	err = ms.updateMapping(mockMapping1.Cid, 123)
	if err != nil {
		t.Errorf("Error updating mapping: %v", err)
	}

	if rs, ok := ms.chunkRawSize[mockMapping1.Cid]; ok {
		sum := rs + uint64(len(mockMapping1.Cid.Bytes()))
		buf := make([]byte, 8)
		n := binary.PutUvarint(buf, sum)
		offset = offset - uint64(n)
	}

	// Perform assertions to verify the mapping was updated correctly.
	updatedMapping := ms.mappings[mockMapping1.Cid]

	if updatedMapping.DstOffset != 121 {
		t.Errorf("Expected DstOffset to be updated to 121, but got %d", updatedMapping.DstOffset)
	}
}

func TestMappingService_SaveAndLoadMetaMappings(t *testing.T) {
	// Initialize a MappingService instance.
	ms := New( /* options */ )

	c1, err := cid.Decode("bafybeictelraxs64igqaz7drzanu2rdt4kk5veo7jp2gea5g3i3evjkrdq")
	if err != nil {
		t.Errorf("decode cid failed")
	}

	// Create mock ChunkMapping instances for testing.
	mockMapping1 := &types.ChunkMapping{
		SrcPath:   "input/test1.json",
		SrcOffset: 0,
		Size:      93,
		DstOffset: 14987673,
		ChunkSize: 139,
		NodeType:  2,
		Cid:       c1,
	}
	c2, err := cid.Decode("bafybeidh5ub7o3v452t5r66inkntzpyvzzlv77vhxdr52tei26qxka6vke")
	if err != nil {
		t.Errorf("decode cid failed")
	}
	mockMapping2 := &types.ChunkMapping{
		SrcPath:   "input/test2.json",
		SrcOffset: 0,
		Size:      90,
		DstOffset: 14987812,
		ChunkSize: 136,
		NodeType:  2,
		Cid:       c2,
	}

	// Insert the mock mappings into the mappings map for testing.
	ms.mappings[mockMapping1.Cid] = mockMapping1
	ms.mappings[mockMapping2.Cid] = mockMapping2

	// Create a temporary directory for testing.
	tempDir, err := ioutil.TempDir("", "test-mappings")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Call SaveMetaMappings to save the mappings to a file.
	metaFileName := "test-meta.json"
	err = ms.SaveMetaMappings(tempDir, metaFileName)
	if err != nil {
		t.Fatalf("Error saving meta mappings: %v", err)
	}

	// Call LoadMetaMappings to load mappings from the saved file.
	err = ms.LoadMetaMappings(filepath.Join(tempDir, metaFileName))
	if err != nil {
		t.Fatalf("Error loading meta mappings: %v", err)
	}

	// Check if the loaded mappings match the original mock mappings.
	// You can perform assertions to compare the loaded mappings with the expected mappings.
	assert.Equal(t, len(ms.mappings), 2, "Loaded mappings count doesn't match expected count")

	// Add more assertions based on your specific data and use case.
}

func TestMappingService_GetChunkMappings(t *testing.T) {
	// Create a new instance of MappingService
	ms := New() // You might need to provide any required options

	if err := ms.LoadMetaMappings("../testdata/output/metas/bafybeiekw7iaz4zjgfq3gdcyh2zh77m3j5ns75w7lyu5nqq3bgoccjgzmq.json"); err != nil {
		t.Fatalf("Failed to load mappings: %v", err)
	}
	// Call the GetChunkMappings function with test data
	dstOffset := uint64(15653)

	dstSize := uint64(140)
	mappings, err := ms.GetChunkMappings(dstOffset, dstSize)
	if err != nil {
		t.Fatalf("Failed to get chunks : %v", err)
	}
	if len(mappings) != 4 {
		t.Fatalf("Failed to get chunks num: 4 ,but  %d", len(mappings))
	}
}

func TestMappingService_GenerateChunksFromMappings(t *testing.T) {
	// Create a new instance of MappingService
	ms := New() // You might need to provide any required options

	if err := ms.LoadMetaMappings("../testdata/output/metas/bafybeiekw7iaz4zjgfq3gdcyh2zh77m3j5ns75w7lyu5nqq3bgoccjgzmq.json"); err != nil {
		t.Fatalf("Failed to load mappings: %v", err)
	}

	// Set up the test parameters
	path := "../testdata/test_output.car"
	srcParent := "../testdata"
	mappings, err := ms.GetAllChunkMappings()
	if err != nil {
		t.Fatalf("Failed to get chunks: %v", err)
	}

	// Call the GenerateChunksFromMappings function with test data
	err = ms.GenerateChunksFromMappings(path, srcParent, mappings)
	if err != nil {
		t.Fatalf("Failed to generate chunks car: %v", err)
	}
}
