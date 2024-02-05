package interop_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/go-yaml/yaml"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

type TestCase struct {
	Seed string `yaml:"seed"`
}

type KeyTest struct {
	TestCases []*TestCase `yaml:"test_cases"`
}

func TestKeyGenerator(t *testing.T) {
	path, err := bazel.Runfile("keygen_test_vector.yaml")
	require.NoError(t, err)
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	testCases := &KeyTest{}
	require.NoError(t, yaml.Unmarshal(file, testCases))
	seeds, pubkeys, err := interop.DeterministicallyGenerateKeys(0, 100)
	require.NoError(t, err)
	// cross-check with the first 100 keys generated from the python spec
	for i, s := range seeds {
		hexSeed := testCases.TestCases[i].Seed
		nKey, err := hexutil.Decode("0x" + hexSeed)
		if err != nil {
			t.Error(err)
			continue
		}
		assert.DeepEqual(t, s.Marshal(), nKey)
		fmt.Printf("pubkey: %s seed: %s \n", hexutil.Encode(pubkeys[i].Marshal()), hexutil.Encode(s.Marshal()))
	}
}
