package history_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/db/kv"
	dbtest "github.com/theQRL/qrysm/validator/db/testing"
	history "github.com/theQRL/qrysm/validator/slashing-protection-history"
	"github.com/theQRL/qrysm/validator/slashing-protection-history/format"
	slashtest "github.com/theQRL/qrysm/validator/testing"
)

func TestImportExport_RoundTrip(t *testing.T) {
	ctx := context.Background()
	numValidators := 10
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := slashtest.MockAttestingAndProposalHistories(publicKeys)
	require.NoError(t, err)
	wanted, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(wanted)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	require.NoError(t, err)

	rawPublicKeys := make([][]byte, numValidators)
	for i := range numValidators {
		rawPublicKeys[i] = publicKeys[i][:]
	}

	// Next up, we export our slashing protection database into the EIP standard file.
	// Next, we attempt to import it into our validator database.
	eipStandard, err := history.ExportStandardProtectionJSON(ctx, validatorDB)
	require.NoError(t, err)

	// We compare the metadata fields from import to export.
	require.Equal(t, wanted.Metadata, eipStandard.Metadata)

	// The values in the data field of the EIP struct are not guaranteed to be sorted,
	// so we create a map to verify we have the data we expected.
	require.Equal(t, len(wanted.Data), len(eipStandard.Data))

	dataByPubKey := make(map[string]*format.ProtectionData)
	for _, item := range wanted.Data {
		dataByPubKey[item.Pubkey] = item
	}
	for _, item := range eipStandard.Data {
		want, ok := dataByPubKey[item.Pubkey]
		require.Equal(t, true, ok)
		require.Equal(t, len(want.SignedAttestations), len(item.SignedAttestations))
		require.Equal(t, len(want.SignedBlocks), len(item.SignedBlocks))
		wantedAttsByRoot := make(map[string]*format.SignedAttestation)
		for _, att := range want.SignedAttestations {
			wantedAttsByRoot[att.SigningRoot] = att
		}
		for _, att := range item.SignedAttestations {
			wantedAtt, ok := wantedAttsByRoot[att.SigningRoot]
			require.Equal(t, true, ok)
			require.DeepEqual(t, wantedAtt, att)
		}
		require.DeepEqual(t, want.SignedBlocks, item.SignedBlocks)
	}
}

func TestImportExport_RoundTrip_SkippedAttestationEpochs(t *testing.T) {
	ctx := context.Background()
	numValidators := 1
	pubKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, pubKeys)
	wanted := &format.EIPSlashingProtectionFormat{
		Metadata: struct {
			InterchangeFormatVersion string `json:"interchange_format_version"`
			GenesisValidatorsRoot    string `json:"genesis_validators_root"`
		}{
			InterchangeFormatVersion: format.InterchangeFormatVersion,
			GenesisValidatorsRoot:    fmt.Sprintf("%#x", [32]byte{1}),
		},
		Data: []*format.ProtectionData{
			{
				Pubkey: fmt.Sprintf("%#x", pubKeys[0]),
				SignedAttestations: []*format.SignedAttestation{
					{
						SourceEpoch: "1",
						TargetEpoch: "2",
					},
					{
						SourceEpoch: "8",
						TargetEpoch: "9",
					},
				},
				SignedBlocks: make([]*format.SignedBlock, 0),
			},
		},
	}
	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(wanted)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	require.NoError(t, err)

	rawPublicKeys := make([][]byte, numValidators)
	for i := range numValidators {
		rawPublicKeys[i] = pubKeys[i][:]
	}

	// Next up, we export our slashing protection database into the EIP standard file.
	// Next, we attempt to import it into our validator database.
	eipStandard, err := history.ExportStandardProtectionJSON(ctx, validatorDB)
	require.NoError(t, err)

	// We compare the metadata fields from import to export.
	require.Equal(t, wanted.Metadata, eipStandard.Metadata)

	// The values in the data field of the EIP struct are not guaranteed to be sorted,
	// so we create a map to verify we have the data we expected.
	require.Equal(t, len(wanted.Data), len(eipStandard.Data))
	require.DeepEqual(t, wanted.Data, eipStandard.Data)
}

func TestImportExport_FilterKeys(t *testing.T) {
	ctx := context.Background()
	numValidators := 10
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := slashtest.MockAttestingAndProposalHistories(publicKeys)
	require.NoError(t, err)
	wanted, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(wanted)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	require.NoError(t, err)

	// Next up, we export our slashing protection database into the EIP standard file.
	// Next, we attempt to import it into our validator database.
	rawKeys := make([][]byte, 5)
	for i := range rawKeys {
		rawKeys[i] = publicKeys[i][:]
	}

	eipStandard, err := history.ExportStandardProtectionJSON(ctx, validatorDB, rawKeys...)
	require.NoError(t, err)

	// We compare the metadata fields from import to export.
	require.Equal(t, wanted.Metadata, eipStandard.Metadata)
	require.Equal(t, len(rawKeys), len(eipStandard.Data))
}

func TestImportInterchangeData_OK(t *testing.T) {
	ctx := context.Background()
	numValidators := 10
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := slashtest.MockAttestingAndProposalHistories(publicKeys)
	require.NoError(t, err)
	standardProtectionFormat, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	require.NoError(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify those indeed match the originally generated mock histories.
	for i := range publicKeys {
		receivedAttestingHistory, err := validatorDB.AttestationHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)

		wantedAttsByRoot := make(map[[32]byte]*kv.AttestationRecord)
		for _, att := range attestingHistory[i] {
			var signingRoot [32]byte
			copy(signingRoot[:], att.SigningRoot)
			wantedAttsByRoot[signingRoot] = att
		}
		for _, att := range receivedAttestingHistory {
			var signingRoot [32]byte
			copy(signingRoot[:], att.SigningRoot)
			wantedAtt, ok := wantedAttsByRoot[signingRoot]
			require.Equal(t, true, ok)
			require.DeepEqual(t, wantedAtt, att)
		}

		proposals := proposalHistory[i].Proposals
		receivedProposalHistory, err := validatorDB.ProposalHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)
		rootsBySlot := make(map[primitives.Slot][]byte)
		for _, proposal := range receivedProposalHistory {
			rootsBySlot[proposal.Slot] = proposal.SigningRoot
		}
		for _, proposal := range proposals {
			receivedRoot, ok := rootsBySlot[proposal.Slot]
			require.DeepEqual(t, true, ok)
			require.DeepEqual(
				t,
				receivedRoot,
				proposal.SigningRoot,
				"Imported proposals are different then the generated ones",
			)
		}
	}
}

func TestImportInterchangeData_OK_SavesBlacklistedPublicKeys(t *testing.T) {
	ctx := context.Background()
	numValidators := 3
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := slashtest.MockAttestingAndProposalHistories(publicKeys)
	require.NoError(t, err)

	standardProtectionFormat, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We add a slashable block for public key at index 1.
	pubKey0 := standardProtectionFormat.Data[0].Pubkey
	standardProtectionFormat.Data[0].SignedBlocks = append(
		standardProtectionFormat.Data[0].SignedBlocks,
		&format.SignedBlock{
			Slot:        "700",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{1}),
		},
		&format.SignedBlock{
			Slot:        "700",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{2}),
		},
	)

	// We add a slashable attestation for public key at index 1
	// representing a double vote event.
	pubKey1 := standardProtectionFormat.Data[1].Pubkey
	standardProtectionFormat.Data[1].SignedAttestations = append(
		standardProtectionFormat.Data[1].SignedAttestations,
		&format.SignedAttestation{
			TargetEpoch: "700",
			SourceEpoch: "699",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{1}),
		},
		&format.SignedAttestation{
			TargetEpoch: "700",
			SourceEpoch: "699",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{2}),
		},
	)

	// We add a slashable attestation for public key at index 2
	// representing a surround vote event.
	pubKey2 := standardProtectionFormat.Data[2].Pubkey
	standardProtectionFormat.Data[2].SignedAttestations = append(
		standardProtectionFormat.Data[2].SignedAttestations,
		&format.SignedAttestation{
			TargetEpoch: "800",
			SourceEpoch: "805",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{4}),
		},
		&format.SignedAttestation{
			TargetEpoch: "801",
			SourceEpoch: "804",
			SigningRoot: fmt.Sprintf("%#x", [32]byte{5}),
		},
	)

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	require.NoError(t, err)

	// Assert the three slashable keys in the imported JSON were saved to the database.
	sKeys, err := validatorDB.EIPImportBlacklistedPublicKeys(ctx)
	require.NoError(t, err)
	slashableKeys := make(map[string]bool)
	for _, pubKey := range sKeys {
		pkString := fmt.Sprintf("%#x", pubKey)
		slashableKeys[pkString] = true
	}
	ok := slashableKeys[pubKey0]
	assert.Equal(t, true, ok)
	ok = slashableKeys[pubKey1]
	assert.Equal(t, true, ok)
	ok = slashableKeys[pubKey2]
	assert.Equal(t, true, ok)
}

func TestStore_ImportInterchangeData_BadFormat_PreventsDBWrites(t *testing.T) {
	ctx := context.Background()
	numValidators := 5
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := slashtest.MockAttestingAndProposalHistories(publicKeys)
	require.NoError(t, err)
	standardProtectionFormat, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We replace a slot of one of the blocks with junk data.
	standardProtectionFormat.Data[0].SignedBlocks[0].Slot = "BadSlot"

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database and check that
	// we obtain an error during the import process.
	err = history.ImportStandardProtectionJSON(ctx, validatorDB, buf)
	assert.NotNil(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify nothing was saved to the DB. If there is an error in the import process, we need to make
	// sure writing is an atomic operation: either the import succeeds and saves the slashing protection
	// data to our DB, or it does not.
	for i := range publicKeys {
		receivedAttestingHistory, err := validatorDB.AttestationHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)
		require.Equal(
			t,
			0,
			len(receivedAttestingHistory),
			"Imported attestation protection history is different than the empty default",
		)
		receivedHistory, err := validatorDB.ProposalHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)
		require.DeepEqual(
			t,
			make([]*kv.Proposal, 0),
			receivedHistory,
			"Imported proposal signing root is different than the empty default",
		)
	}
}

// Regression test for the qrysm port of upstream PR #13232: a public key that
// is slashable with respect to attestations already in the database must be
// blacklisted without aborting the whole import - other keys' histories must
// still be imported. The import previously returned an error for the entire
// file as soon as any key conflicted with the local database.
func TestImportInterchangeData_ContinuesOnKeySlashableVersusDB(t *testing.T) {
	ctx := context.Background()
	numValidators := 2
	publicKeys, err := slashtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := dbtest.SetupDB(t, publicKeys)

	// Build a deterministic protection JSON: each key attests (source 0,
	// target 1) and proposes at slot 1 with its own signing root.
	attestingHistory := make([][]*kv.AttestationRecord, numValidators)
	proposalHistory := make([]kv.ProposalHistoryForPubkey, numValidators)
	for v := range numValidators {
		var signingRoot [32]byte
		signingRoot[0] = byte(v + 1)
		attestingHistory[v] = []*kv.AttestationRecord{
			{Source: 0, Target: 1, SigningRoot: signingRoot[:], PubKey: publicKeys[v]},
		}
		proposalHistory[v] = kv.ProposalHistoryForPubkey{
			Proposals: []kv.Proposal{{Slot: 1, SigningRoot: signingRoot[:]}},
		}
	}
	standardProtectionFormat, err := slashtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// Pre-populate the database with an attestation for key 0 at the same
	// target epoch but a different signing root - a double vote with respect
	// to the database.
	existingAtt := &qrysmpb.IndexedAttestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Epoch: 0},
			Target: &qrysmpb.Checkpoint{Epoch: 1},
		},
	}
	require.NoError(t, validatorDB.SaveAttestationForPubKey(ctx, publicKeys[0], [32]byte{99}, existingAtt))

	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// The import must complete instead of aborting on the conflicting key.
	require.NoError(t, history.ImportStandardProtectionJSON(ctx, validatorDB, buf))

	// Key 0 is blacklisted, key 1 is not.
	sKeys, err := validatorDB.EIPImportBlacklistedPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(sKeys))
	require.DeepEqual(t, publicKeys[0], sKeys[0])

	// Key 1's history was imported.
	proposals, err := validatorDB.ProposalHistoryForPubKey(ctx, publicKeys[1])
	require.NoError(t, err)
	require.Equal(t, true, len(proposals) > 0, "expected key 1's proposal history to be imported")
}
