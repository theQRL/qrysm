package randao

import (
	"crypto/sha256"
	"testing"

	"github.com/theQRL/qrysm/testing/require"
)

func TestCommitmentMatchesManualChain(t *testing.T) {
	seed := []byte("seed-a")
	x := sha256.Sum256(append([]byte("qrl-randao-onion-v1"), seed...))
	for range 5 {
		x = sha256.Sum256(x[:])
	}
	require.Equal(t, x, Commitment(seed, 5))

	o, err := NewOnion(seed, 5)
	require.NoError(t, err)
	require.Equal(t, x, o.Commitment())
	require.Equal(t, uint64(5), o.Layers())
}

func TestRevealWalksDownTheChain(t *testing.T) {
	const layers = 3000 // spans several checkpoints
	o, err := NewOnion([]byte("seed-b"), layers)
	require.NoError(t, err)

	commitment := o.Commitment()
	for i := 0; i < layers; i++ {
		reveal, err := o.Reveal(commitment)
		require.NoError(t, err, "layer %d", i)
		require.Equal(t, true, Verify(reveal, commitment))
		commitment = reveal
	}
	_, err = o.Reveal(commitment)
	require.ErrorIs(t, err, ErrExhausted)
}

func TestRevealUnknownCommitment(t *testing.T) {
	o, err := NewOnion([]byte("seed-c"), 64)
	require.NoError(t, err)
	_, err = o.Reveal(Commitment([]byte("other-seed"), 64))
	require.ErrorIs(t, err, ErrUnknownCommitment)
}

func TestRevealAfterReorgUsesHintWindow(t *testing.T) {
	o, err := NewOnion([]byte("seed-d"), 2048)
	require.NoError(t, err)

	top := o.Commitment()
	r1, err := o.Reveal(top)
	require.NoError(t, err)
	r2, err := o.Reveal(r1)
	require.NoError(t, err)
	// Both blocks orphaned: the head state still shows the original commitment.
	again, err := o.Reveal(top)
	require.NoError(t, err)
	require.Equal(t, r1, again)
	// Only the first was included.
	again, err = o.Reveal(r1)
	require.NoError(t, err)
	require.Equal(t, r2, again)
}

func TestRevealDoesNotDependOnLayersKnownToClient(t *testing.T) {
	// The deposit was made with a short onion; a client built with a longer
	// one from the same seed still finds the pre-image.
	seed := []byte("seed-e")
	short := Commitment(seed, 10)
	long, err := NewOnion(seed, 5000)
	require.NoError(t, err)
	reveal, err := long.Reveal(short)
	require.NoError(t, err)
	require.Equal(t, true, Verify(reveal, short))
	require.Equal(t, Commitment(seed, 9), reveal)
}

func TestNewOnionZeroLayers(t *testing.T) {
	_, err := NewOnion([]byte("x"), 0)
	require.ErrorIs(t, err, ErrInvalidLayers)
}

func TestLayerBeyondLength(t *testing.T) {
	o, err := NewOnion([]byte("x"), 4)
	require.NoError(t, err)
	_, err = o.Layer(5)
	require.NotNil(t, err)
}
