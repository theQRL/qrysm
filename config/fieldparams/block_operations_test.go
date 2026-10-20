package field_params_test

import (
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestBlockOperationLimitsMatchSSZ(t *testing.T) {
	// These fixtures need valid SSZ shapes, not cryptographic signatures.
	header := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{})
	proposerSlashing := &qrysmpb.ProposerSlashing{Header_1: header, Header_2: header}
	indexedAtt := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{AttestingIndices: []uint64{0}})
	attesterSlashing := &qrysmpb.AttesterSlashing{Attestation_1: indexedAtt, Attestation_2: indexedAtt}
	att := util.HydrateAttestation(&qrysmpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}})
	deposit := &qrysmpb.Deposit{
		Proof: make([][]byte, fieldparams.DepositProofLength),
		Data: &qrysmpb.Deposit_Data{
			PublicKey:           make([]byte, fieldparams.MLDSA87PubkeyLength),
			WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
			RandaoCommitment:    make([]byte, fieldparams.RandaoCommitmentLength),
			Signature:           make([]byte, fieldparams.MLDSA87SignatureLength),
		},
	}
	for i := range deposit.Proof {
		deposit.Proof[i] = make([]byte, fieldparams.RootLength)
	}
	exit := &qrysmpb.SignedVoluntaryExit{Exit: &qrysmpb.VoluntaryExit{}, Signature: make([]byte, fieldparams.MLDSA87SignatureLength)}
	withdrawal := &enginev1.Withdrawal{Address: make([]byte, fieldparams.WithdrawalRecipientLength)}

	for _, tc := range []struct {
		name  string
		limit uint64
		add   func(*qrysmpb.BeaconBlockBodyZond)
	}{
		{"ProposerSlashings", fieldparams.MaxProposerSlashings, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.ProposerSlashings = append(b.ProposerSlashings, proposerSlashing)
		}},
		{"AttesterSlashings", fieldparams.MaxAttesterSlashings, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.AttesterSlashings = append(b.AttesterSlashings, attesterSlashing)
		}},
		{"Attestations", fieldparams.MaxAttestations, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.Attestations = append(b.Attestations, att)
		}},
		{"Deposits", fieldparams.MaxDeposits, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.Deposits = append(b.Deposits, deposit)
		}},
		{"VoluntaryExits", fieldparams.MaxVoluntaryExits, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.VoluntaryExits = append(b.VoluntaryExits, exit)
		}},
		{"Withdrawals", fieldparams.MaxWithdrawalsPerPayload, func(b *qrysmpb.BeaconBlockBodyZond) {
			b.ExecutionPayload.Withdrawals = append(b.ExecutionPayload.Withdrawals, withdrawal)
		}},
	} {
		for _, count := range []uint64{0, tc.limit, tc.limit + 1} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, count), func(t *testing.T) {
				body := util.HydrateBeaconBlockBodyZond(nil)
				for range count {
					tc.add(body)
				}
				t.Run("full block body", func(t *testing.T) {
					checkBlockOperationSSZBound(t, body, new(qrysmpb.BeaconBlockBodyZond), tc.name, count > tc.limit)
				})
				if tc.name == "Withdrawals" {
					t.Run("execution payload", func(t *testing.T) {
						checkBlockOperationSSZBound(t, body.ExecutionPayload, new(enginev1.ExecutionPayloadZond), tc.name, count > tc.limit)
					})
					return
				}
				t.Run("blinded block body", func(t *testing.T) {
					blinded := util.HydrateBlindedBeaconBlockBodyZond(nil)
					blinded.ProposerSlashings = body.ProposerSlashings
					blinded.AttesterSlashings = body.AttesterSlashings
					blinded.Attestations = body.Attestations
					blinded.Deposits = body.Deposits
					blinded.VoluntaryExits = body.VoluntaryExits
					checkBlockOperationSSZBound(t, blinded, new(qrysmpb.BlindedBeaconBlockBodyZond), tc.name, count > tc.limit)
				})
			})
		}
	}
}

type blockOperationSSZ interface {
	MarshalSSZ() ([]byte, error)
	UnmarshalSSZ([]byte) error
	HashTreeRoot() ([32]byte, error)
}

func checkBlockOperationSSZBound(t *testing.T, value, decoded blockOperationSSZ, field string, tooLarge bool) {
	t.Helper()
	encoded, marshalErr := value.MarshalSSZ()
	root, hashErr := value.HashTreeRoot()
	if tooLarge {
		require.ErrorContains(t, field, marshalErr)
		require.NotNil(t, hashErr, "hashing must reject a list above the compiled limit")
		return
	}
	require.NoError(t, marshalErr)
	require.NoError(t, hashErr)
	require.NoError(t, decoded.UnmarshalSSZ(encoded))
	decodedRoot, err := decoded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, root, decodedRoot)
	roundTrip, err := decoded.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, encoded, roundTrip)
}
