package keyderivation

import (
	"strings"
	"testing"

	goqrllib_misc "github.com/theQRL/go-qrllib/wallet/misc"
	"github.com/theQRL/qrysm/testing/require"
)

func TestGetRandomMnemonic(t *testing.T) {
	first, err := GetRandomMnemonic()
	require.NoError(t, err)
	second, err := GetRandomMnemonic()
	require.NoError(t, err)
	require.NotEqual(t, first, second, "two generated mnemonics must not be equal")

	require.Equal(t, 32, len(strings.Split(first, " ")))

	// The mnemonic must decode to a full-size seed and re-encode to itself, i.e.
	// it is produced with the same word mapping used by `deposit new-seed` to
	// turn it back into the validator seed.
	seed, err := goqrllib_misc.MnemonicToBin(first)
	require.NoError(t, err)
	require.Equal(t, mnemonicSeedSize, len(seed))
	roundTrip, err := goqrllib_misc.BinToMnemonic(seed)
	require.NoError(t, err)
	require.Equal(t, first, roundTrip)
}
