package ssz_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/encoding/ssz"
	"github.com/theQRL/qrysm/testing/assert"
)

func TestGetDepth(t *testing.T) {
	trieSize := uint64(896745231)
	expected := uint8(30)

	result := ssz.Depth(trieSize)
	assert.Equal(t, expected, result)
}

func TestMerkleizeCountGreaterThanLimit(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(2)
	limit := uint64(1)
	chunks := [][]byte{{}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	// Error if no panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic.")
		}
	}()
	ssz.Merkleize(hashFn, count, limit, leafIndexer)
}

func TestMerkleizeLimitAndCountAreZero(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(0)
	limit := uint64(0)
	chunks := [][]byte{{}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	var expected [32]byte
	result := ssz.Merkleize(hashFn, count, limit, leafIndexer)
	assert.Equal(t, expected, result)
}

func TestMerkleizeNormalPath(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(2)
	limit := uint64(3)
	chunks := [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	expected := [32]byte{95, 27, 253, 237, 215, 58, 147, 198, 175, 194, 180, 231, 154, 130, 205, 68, 146, 112, 225, 86, 6, 103, 186, 82, 7, 142, 33, 189, 174, 56, 199, 173}
	result := ssz.Merkleize(hashFn, count, limit, leafIndexer)
	assert.Equal(t, expected, result)
}

func TestConstructProofCountGreaterThanLimit(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(2)
	limit := uint64(1)
	chunks := [][]byte{{}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	index := uint64(0)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic.")
		}
	}()
	ssz.ConstructProof(hashFn, count, limit, leafIndexer, index)
}

func TestConstructProofIndexGreaterThanEqualToLimit(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(1)
	limit := uint64(1)
	chunks := [][]byte{{}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	index := uint64(1)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic.")
		}
	}()
	ssz.ConstructProof(hashFn, count, limit, leafIndexer, index)
}

func TestConstructProofNormalPath(t *testing.T) {
	hashFn := ssz.NewHasherFunc(hash.CustomSHA256Hasher())
	count := uint64(2)
	limit := uint64(3)
	chunks := [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}}
	leafIndexer := func(i uint64) []byte {
		return chunks[i]
	}
	index := uint64(1)
	expected := [][32]byte{
		{1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{245, 165, 253, 66, 209, 106, 32, 48, 39, 152, 239, 110, 211, 9, 151, 155, 67, 0, 61, 35, 32, 217, 240, 232, 234, 152, 49, 169, 39, 89, 251, 75},
	}
	result := ssz.ConstructProof(hashFn, count, limit, leafIndexer, index)
	assert.Equal(t, len(expected), len(result))
	for i, v := range expected {
		assert.DeepEqual(t, result[i], v)
	}
}

func TestDepthOfOne(t *testing.T) {
	assert.Equal(t, uint8(0), ssz.Depth(1))
}

func TestMerkleizeAndProof_LeafLengths(t *testing.T) {
	pair := func(a, b [32]byte) [32]byte {
		var input [64]byte
		copy(input[:32], a[:])
		copy(input[32:], b[:])
		return sha256.Sum256(input[:])
	}
	for _, lengths := range []struct {
		name  string
		sizes []int
	}{
		{"full", []int{32}},
		{"short", []int{4}},
		{"mixed", []int{32, 0, 1, 31, 4, 32, 17, 8}},
	} {
		for _, limit := range []int{1, 2, 3, 4, 7, 8, 15, 16, 17, 31, 32, 33, 65} {
			for count := 0; count <= limit; count++ {
				t.Run(fmt.Sprintf("%s/limit=%d/count=%d", lengths.name, limit, count), func(t *testing.T) {
					leaves := make([][]byte, count)
					width := 1
					for width < limit {
						width *= 2
					}
					layer := make([][32]byte, width)
					for i := range leaves {
						leaves[i] = make([]byte, lengths.sizes[i%len(lengths.sizes)])
						for j := range leaves[i] {
							leaves[i][j] = byte(i + j + 1)
						}
						copy(layer[i][:], leaves[i])
					}
					// Build the expected tree independently, with explicit zero padding.
					for len(layer) > 1 {
						next := make([][32]byte, len(layer)/2)
						for i := range next {
							next[i] = pair(layer[2*i], layer[2*i+1])
						}
						layer = next
					}
					want := layer[0]
					hasher := ssz.NewHasherFunc(sha256.Sum256)
					leaf := func(i uint64) []byte { return leaves[i] }
					got := ssz.Merkleize(hasher, uint64(count), uint64(limit), leaf)
					assert.Equal(t, want, got, "merkle root differs")
					for index := 0; index < limit; index++ {
						var root [32]byte
						if index < count {
							copy(root[:], leaves[index])
						}
						proof := ssz.ConstructProof(hasher, uint64(count), uint64(limit), leaf, uint64(index))
						for depth, sibling := range proof {
							if (index>>depth)&1 == 0 {
								root = pair(root, sibling)
							} else {
								root = pair(sibling, root)
							}
						}
						if root != want {
							t.Errorf("proof for index %d does not reconstruct the root", index)
						}
					}
				})
			}
		}
	}
}

func TestMerkleizeVector_DoesNotWriteIntoCallerArray(t *testing.T) {
	leaves := make([][32]byte, 4)
	for i := range leaves {
		leaves[i][0] = byte(i + 1)
	}
	fourth := leaves[3]
	got := ssz.MerkleizeVector(leaves[:3], 4)
	assert.Equal(t, fourth, leaves[3], "padding must not overwrite the caller's spare capacity")
	padded := make([][32]byte, 4)
	copy(padded, leaves[:3])
	assert.Equal(t, ssz.MerkleizeVector(padded, 4), got)
}
