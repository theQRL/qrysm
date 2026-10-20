package ssz_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/encoding/ssz"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

const merkleizingListLimitError = "merkleizing list that is too large, over limit"

func TestBitlistRoot(t *testing.T) {
	capacity := uint64(10)
	bfield := bitfield.NewBitlist(capacity)
	expected := [32]byte{176, 76, 194, 203, 142, 166, 117, 79, 148, 194, 231, 64, 60, 245, 142, 32, 201, 2, 58, 152, 53, 12, 132, 40, 41, 102, 224, 189, 103, 41, 211, 202}

	result, err := ssz.BitlistRoot(bfield, capacity)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestBitlistRoot_ExactCapacity(t *testing.T) {
	for _, capacity := range []uint64{0, 1, 32, 255, 256, 257} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			_, err := ssz.BitlistRoot(bitfield.NewBitlist(capacity), capacity)
			require.NoError(t, err)
			_, err = ssz.BitlistRoot(bitfield.NewBitlist(capacity+1), capacity)
			assert.ErrorContains(t, "bitlist exceeds maximum capacity", err)
		})
	}
}

func TestBitwiseMerkleizeOverLimit(t *testing.T) {
	chunks := [][32]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}
	count := uint64(2)
	limit := uint64(1)

	_, err := ssz.BitwiseMerkleize(chunks, count, limit)
	assert.ErrorContains(t, merkleizingListLimitError, err)
}

func TestBitwiseMerkleizeArrays(t *testing.T) {
	chunks := [][32]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		{33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 62, 62, 63, 64},
	}
	count := uint64(2)
	limit := uint64(2)
	expected := [32]byte{138, 81, 210, 194, 151, 231, 249, 241, 64, 118, 209, 58, 145, 109, 225, 89, 118, 110, 159, 220, 193, 183, 203, 124, 166, 24, 65, 26, 160, 215, 233, 219}

	result, err := ssz.BitwiseMerkleize(chunks, count, limit)
	require.NoError(t, err)
	assert.Equal(t, expected, result)

	chunks = [][32]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	}
	count = uint64(2)
	limit = uint64(2)
	expected = [32]byte{194, 32, 213, 52, 220, 127, 18, 240, 43, 151, 19, 79, 188, 175, 142, 177, 208, 46, 96, 20, 18, 231, 208, 29, 120, 102, 122, 17, 46, 31, 155, 30}

	result, err = ssz.BitwiseMerkleize(chunks, count, limit)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestBitwiseMerkleizeArraysOverLimit(t *testing.T) {
	chunks := [][32]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		{33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 62, 62, 63, 64},
	}
	count := uint64(2)
	limit := uint64(1)

	_, err := ssz.BitwiseMerkleize(chunks, count, limit)
	assert.ErrorContains(t, merkleizingListLimitError, err)
}

func TestPackByChunk(t *testing.T) {
	byteSlice2D := [][]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 2, 5, 2, 6, 2, 7},
		{1, 1, 2, 3, 5, 8, 13, 21, 34},
	}
	expected := [][32]byte{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 2, 5, 2, 6, 2, 7, 1, 1},
		{2, 3, 5, 8, 13, 21, 34, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}

	result, err := ssz.PackByChunk(byteSlice2D)
	require.NoError(t, err)
	assert.Equal(t, len(expected), len(result))
	for i, v := range expected {
		assert.DeepEqual(t, v, result[i])
	}
}

func TestPackByChunk_MixedSizes(t *testing.T) {
	for _, sizes := range [][]int{{32, 33}, {32, 1, 31}, {32, 0, 32}, {32, 32}} {
		t.Run(fmt.Sprint(sizes), func(t *testing.T) {
			items := make([][]byte, len(sizes))
			var flat []byte
			for i, size := range sizes {
				items[i] = bytes.Repeat([]byte{byte(i + 1)}, size)
				flat = append(flat, items[i]...)
			}
			chunks, err := ssz.PackByChunk(items)
			require.NoError(t, err)
			require.Equal(t, (len(flat)+31)/32, len(chunks))
			var packed []byte
			for _, chunk := range chunks {
				packed = append(packed, chunk[:]...)
			}
			assert.DeepEqual(t, flat, packed[:len(flat)])
			assert.DeepEqual(t, make([]byte, len(packed)-len(flat)), packed[len(flat):])
		})
	}
}

func TestMixInLength(t *testing.T) {
	byteSlice := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	length := []byte{1, 2, 3}
	expected := [32]byte{105, 60, 167, 169, 197, 220, 122, 99, 59, 14, 250, 12, 251, 62, 135, 239, 29, 68, 140, 1, 6, 36, 207, 44, 64, 221, 76, 230, 237, 218, 150, 88}
	result := ssz.MixInLength(byteSlice, length)
	assert.Equal(t, expected, result)
}

func TestBitlistRoot_RejectsMalformed(t *testing.T) {
	cases := map[string]bitfield.Bitlist{
		"typed nil":           nil,
		"empty":               {},
		"zero final byte":     {0xff, 0x00},
		"oversized zero tail": {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00},
	}
	for name, bl := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ssz.BitlistRoot(bl, 32)
			assert.ErrorContains(t, "bitlist has no length bit", err)
		})
	}
	_, err := ssz.BitlistRoot(nil, 32)
	assert.ErrorContains(t, "bitlist is nil", err)
	_, err = ssz.BitlistRoot(bitfield.NewBitlist(0), 32)
	require.NoError(t, err)

	// Pointers satisfy the interface too and must get the same validation.
	_, err = ssz.BitlistRoot((*bitfield.Bitlist)(nil), 32)
	assert.ErrorContains(t, "bitlist is nil", err)
	_, err = ssz.BitlistRoot(&bitfield.Bitlist{0xff, 0x00}, 32)
	assert.ErrorContains(t, "bitlist has no length bit", err)
	_, err = ssz.BitlistRoot(&bitfield.Bitlist{}, 32)
	assert.ErrorContains(t, "bitlist has no length bit", err)
	valid := bitfield.NewBitlist(5)
	valid.SetBitAt(2, true)
	wantRoot, err := ssz.BitlistRoot(valid, 32)
	require.NoError(t, err)
	gotRoot, err := ssz.BitlistRoot(&valid, 32)
	require.NoError(t, err)
	assert.Equal(t, wantRoot, gotRoot)
}

func TestBitwiseMerkleize_RejectsCountMismatch(t *testing.T) {
	chunks := make([][32]byte, 5)
	for i := range chunks {
		chunks[i][0] = byte(i + 1)
	}
	_, err := ssz.BitwiseMerkleize(chunks, 3, 8)
	assert.ErrorContains(t, "does not match", err)
	_, err = ssz.BitwiseMerkleize(chunks, 4, 4)
	assert.ErrorContains(t, "does not match", err)
	_, err = ssz.BitwiseMerkleize(chunks, 5, 8)
	require.NoError(t, err)
}
