package testnet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/runtime/interop"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func Test_genesisStateFromJSONValidators(t *testing.T) {
	numKeys := 5
	jsonData, err := createGenesisDepositData(t, numKeys)
	require.NoError(t, err)
	jsonInput, err := json.Marshal(jsonData)
	require.NoError(t, err)
	_, dds, err := depositEntriesFromJSON(jsonInput)
	require.NoError(t, err)
	for i := range dds {
		assert.DeepEqual(t, fmt.Sprintf("%#x", dds[i].PublicKey), jsonData[i].PubKey)
	}
}

func TestGenerateGenesis_MissingBaseFeeRejected(t *testing.T) {
	// Write a gqrl genesis.json that has every field core.Genesis flags as
	// required (gasLimit, alloc) but deliberately omits baseFeePerGas, so the
	// nil-baseFee guard is what trips.
	dir := t.TempDir()
	genesisPath := filepath.Join(dir, "genesis.json")
	require.NoError(t, os.WriteFile(genesisPath, []byte(`{"config":{},"gasLimit":"0x1c9c380","alloc":{}}`), 0o600))

	saved := generateGenesisStateFlags
	t.Cleanup(func() { generateGenesisStateFlags = saved })

	generateGenesisStateFlags.GqrlGenesisJsonIn = genesisPath
	generateGenesisStateFlags.NumValidators = 1
	generateGenesisStateFlags.GenesisTime = 1

	_, err := generateGenesis(context.Background())
	require.ErrorContains(t, "baseFeePerGas must be set", err)
}

func TestGenerateGenesis_TimestampResolution(t *testing.T) {
	// Write a gqrl genesis.json that carries a known Timestamp and a valid
	// BaseFee. We only assert what generateGenesis resolves
	// generateGenesisStateFlags.GenesisTime to — checking the package-level
	// flag avoids running the heavy validator-keygen path while still
	// exercising the JSON-read + now()-fallback + delay logic.
	const jsonTimestamp = uint64(1700000000)
	body := fmt.Sprintf(
		`{"config":{},"gasLimit":"0x1c9c380","alloc":{},"baseFeePerGas":"0x3b9aca00","timestamp":"0x%x"}`,
		jsonTimestamp,
	)

	tests := []struct {
		name             string
		withInput        bool
		genesisTime      uint64
		genesisTimeDelay uint64
		want             uint64
	}{
		{
			name:      "input JSON timestamp used when --genesis-time unset",
			withInput: true,
			want:      jsonTimestamp,
		},
		{
			name:        "explicit --genesis-time wins over JSON timestamp",
			withInput:   true,
			genesisTime: 1600000000,
			want:        1600000000,
		},
		{
			name:             "delay applied on top of JSON timestamp",
			withInput:        true,
			genesisTimeDelay: 100,
			want:             jsonTimestamp + 100,
		},
		{
			name:             "delay applied on top of --genesis-time",
			withInput:        true,
			genesisTime:      1600000000,
			genesisTimeDelay: 50,
			want:             1600000050,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := generateGenesisStateFlags
			t.Cleanup(func() { generateGenesisStateFlags = saved })

			generateGenesisStateFlags.NumValidators = 1
			generateGenesisStateFlags.GenesisTime = tc.genesisTime
			generateGenesisStateFlags.GenesisTimeDelay = tc.genesisTimeDelay
			generateGenesisStateFlags.GqrlGenesisJsonIn = ""
			if tc.withInput {
				dir := t.TempDir()
				path := filepath.Join(dir, "genesis.json")
				require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
				generateGenesisStateFlags.GqrlGenesisJsonIn = path
			}

			// generateGenesis may continue past the timestamp logic and fail
			// (or panic) later in genesis state construction — that's fine,
			// we only care about the resolved GenesisTime side effect.
			func() {
				defer func() { _ = recover() }()
				_, _ = generateGenesis(context.Background())
			}()

			assert.Equal(t, tc.want, generateGenesisStateFlags.GenesisTime)
		})
	}
}

func createGenesisDepositData(t *testing.T, numKeys int) ([]*depositDataJSON, error) {
	pubKeys := make([]ml_dsa_87.PublicKey, numKeys)
	privKeys := make([]ml_dsa_87.MLDSA87Key, numKeys)
	for i := range numKeys {
		randKey, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		privKeys[i] = randKey
		pubKeys[i] = randKey.PublicKey()
	}
	dataList, _, err := interop.DepositDataFromKeys(privKeys, pubKeys)
	require.NoError(t, err)
	jsonData := make([]*depositDataJSON, numKeys)
	for i := range numKeys {
		dataRoot, err := dataList[i].HashTreeRoot()
		require.NoError(t, err)
		jsonData[i] = &depositDataJSON{
			PubKey:                fmt.Sprintf("%#x", dataList[i].PublicKey),
			Amount:                dataList[i].Amount,
			WithdrawalCredentials: fmt.Sprintf("%#x", dataList[i].WithdrawalCredentials),
			DepositDataRoot:       fmt.Sprintf("%#x", dataRoot),
			Signature:             fmt.Sprintf("%#x", dataList[i].Signature),
		}
	}
	return jsonData, nil
}
