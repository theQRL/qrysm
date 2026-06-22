package testing

import (
	"math/rand"
	"testing"

	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time"
)

// BitlistWithAllBitsSet creates list of bitlists with all bits set.
func BitlistWithAllBitsSet(length uint64) bitfield.Bitlist {
	b := bitfield.NewBitlist(length)
	for i := range length {
		b.SetBitAt(i, true)
	}
	return b
}

// BitlistsWithSingleBitSet creates list of bitlists with a single bit set in each.
func BitlistsWithSingleBitSet(n, length uint64) []bitfield.Bitlist {
	lists := make([]bitfield.Bitlist, n)
	for i := range n {
		b := bitfield.NewBitlist(length)
		b.SetBitAt(i%length, true)
		lists[i] = b
	}
	return lists
}

// Bitlists64WithSingleBitSet creates list of bitlists with a single bit set in each.
func Bitlists64WithSingleBitSet(n, length uint64) []*bitfield.Bitlist64 {
	lists := make([]*bitfield.Bitlist64, n)
	for i := range n {
		b := bitfield.NewBitlist64(length)
		b.SetBitAt(i%length, true)
		lists[i] = b
	}
	return lists
}

// BitlistsWithMultipleBitSet creates list of bitlists with random n bits set.
func BitlistsWithMultipleBitSet(t testing.TB, n, length, count uint64) []bitfield.Bitlist {
	seed := time.Now().UnixNano()
	t.Logf("bitlistsWithMultipleBitSet random seed: %v", seed)
	rand.Seed(seed)
	lists := make([]bitfield.Bitlist, n)
	for i := range n {
		b := bitfield.NewBitlist(length)
		keys := rand.Perm(int(length)) // lint:ignore uintcast -- This is safe in test code.
		for _, key := range keys[:count] {
			b.SetBitAt(uint64(key), true)
		}
		lists[i] = b
	}
	return lists
}

// Bitlists64WithMultipleBitSet creates list of bitlists with random n bits set.
func Bitlists64WithMultipleBitSet(t testing.TB, n, length, count uint64) []*bitfield.Bitlist64 {
	seed := time.Now().UnixNano()
	t.Logf("Bitlists64WithMultipleBitSet random seed: %v", seed)
	rand.Seed(seed)
	lists := make([]*bitfield.Bitlist64, n)
	for i := range n {
		b := bitfield.NewBitlist64(length)
		keys := rand.Perm(int(length)) // lint:ignore uintcast -- This is safe in test code.
		for _, key := range keys[:count] {
			b.SetBitAt(uint64(key), true)
		}
		lists[i] = b
	}
	return lists
}

// MakeAttestationsFromBitlists creates list of attestations from list of bitlist.
func MakeAttestationsFromBitlists(bl []bitfield.Bitlist) []*qrysmpb.Attestation {
	atts := make([]*qrysmpb.Attestation, len(bl))
	for i, b := range bl {
		indices := b.BitIndices()
		signatures := make([][]byte, len(indices))
		for i := range indices {
			signatures[i] = make([]byte, field_params.MLDSA87SignatureLength)
		}

		atts[i] = &qrysmpb.Attestation{
			AggregationBits: b,
			Data: &qrysmpb.AttestationData{
				Slot:           42,
				CommitteeIndex: 1,
			},
			Signatures: signatures,
		}
	}
	return atts
}
