package field_params_test

import (
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func testFieldParametersMatchConfig(t *testing.T) {
	require.Equal(t, uint64(params.BeaconConfig().SlotsPerHistoricalRoot), uint64(fieldparams.BlockRootsLength))
	require.Equal(t, uint64(params.BeaconConfig().SlotsPerHistoricalRoot), uint64(fieldparams.StateRootsLength))
	require.Equal(t, params.BeaconConfig().HistoricalRootsLimit, uint64(fieldparams.HistoricalRootsLength))
	require.Equal(t, uint64(params.BeaconConfig().EpochsPerHistoricalVector), uint64(fieldparams.RandaoMixesLength))
	require.Equal(t, params.BeaconConfig().ValidatorRegistryLimit, uint64(fieldparams.ValidatorRegistryLimit))
	require.Equal(t, uint64(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod))), uint64(fieldparams.ExecutionDataVotesLength))
	require.Equal(t, uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().MaxAttestations)), uint64(fieldparams.PreviousEpochAttestationsLength))
	require.Equal(t, uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().MaxAttestations)), uint64(fieldparams.CurrentEpochAttestationsLength))
	require.Equal(t, uint64(params.BeaconConfig().EpochsPerSlashingsVector), uint64(fieldparams.SlashingsLength))
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(fieldparams.SyncCommitteeLength))
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(fieldparams.MaxValidatorsPerCommittee))
	require.Equal(t, params.BeaconConfig().MaxProposerSlashings, uint64(fieldparams.MaxProposerSlashings))
	require.Equal(t, params.BeaconConfig().MaxAttesterSlashings, uint64(fieldparams.MaxAttesterSlashings))
	require.Equal(t, params.BeaconConfig().MaxAttestations, uint64(fieldparams.MaxAttestations))
	require.Equal(t, params.BeaconConfig().MaxDeposits, uint64(fieldparams.MaxDeposits))
	require.Equal(t, params.BeaconConfig().MaxVoluntaryExits, uint64(fieldparams.MaxVoluntaryExits))
	require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(fieldparams.MaxWithdrawalsPerPayload))
	require.Equal(t, params.BeaconConfig().DepositContractTreeDepth, uint64(fieldparams.DepositProofLength-1))
}

func TestAttestationCommitteeLimitMatchesSSZ(t *testing.T) {
	// Exercise the compiled codec, so the validation bound cannot drift away
	// from the bitlist limit in the protobuf schema or generated SSZ code.
	const limit uint64 = fieldparams.MaxValidatorsPerCommittee
	for _, size := range []uint64{limit, limit + 1, 2 * limit} {
		t.Run(fmt.Sprintf("committee_size_%d", size), func(t *testing.T) {
			att := &qrysmpb.Attestation{
				AggregationBits: bitfield.NewBitlist(size),
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Target:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				Signatures: [][]byte{make([]byte, fieldparams.MLDSA87SignatureLength)},
			}
			att.AggregationBits.SetBitAt(0, true)
			encoded, err := att.MarshalSSZ()
			decoded := new(qrysmpb.Attestation)
			if err == nil {
				err = decoded.UnmarshalSSZ(encoded)
			}
			if size > fieldparams.MaxValidatorsPerCommittee {
				require.NotNil(t, err, "committee above the compiled limit must not round-trip")
				return
			}
			require.NoError(t, err)
			require.DeepEqual(t, att, decoded)
		})
	}
}

func TestDepositProofLengthMatchesSSZ(t *testing.T) {
	// Only valid SSZ shapes are needed, not cryptographic keys or signatures.
	data := &qrysmpb.Deposit_Data{
		PublicKey:           make([]byte, fieldparams.MLDSA87PubkeyLength),
		WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
		RandaoCommitment:    make([]byte, fieldparams.RandaoCommitmentLength),
		Signature:           make([]byte, fieldparams.MLDSA87SignatureLength),
	}
	leaf, err := data.HashTreeRoot()
	require.NoError(t, err)
	for _, depth := range []uint64{fieldparams.DepositProofLength - 2, fieldparams.DepositProofLength - 1, fieldparams.DepositProofLength} {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			sparseTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, depth)
			require.NoError(t, err)
			proof, err := sparseTrie.MerkleProof(0)
			require.NoError(t, err)
			require.Equal(t, depth+1, uint64(len(proof)))
			trieRoot, err := sparseTrie.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, true, trie.VerifyMerkleProofWithDepth(trieRoot[:], leaf[:], 0, proof, depth))

			deposit := &qrysmpb.Deposit{Proof: proof, Data: data}
			encoded, marshalErr := deposit.MarshalSSZ()
			depositRoot, hashErr := deposit.HashTreeRoot()
			if len(proof) != fieldparams.DepositProofLength {
				// A valid generic trie proof may still be incompatible with the
				// fixed SSZ vector: shorter proofs are invalid too.
				require.ErrorContains(t, "Proof", marshalErr)
				require.ErrorContains(t, "Proof", hashErr)
				return
			}
			require.NoError(t, marshalErr)
			require.NoError(t, hashErr)
			decoded := new(qrysmpb.Deposit)
			require.NoError(t, decoded.UnmarshalSSZ(encoded))
			require.DeepEqual(t, deposit, decoded)
			decodedRoot, err := decoded.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, depositRoot, decodedRoot)
		})
	}
}
