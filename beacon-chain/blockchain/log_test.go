package blockchain

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_logStateTransitionData(t *testing.T) {
	payloadBlk := &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			SyncAggregate: &zondpb.SyncAggregate{},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				BlockHash:    []byte{1, 2, 3},
				Transactions: [][]byte{{}, {}},
			},
		},
	}
	wrappedPayloadBlk, err := blocks.NewBeaconBlock(payloadBlk)
	require.NoError(t, err)
	tests := []struct {
		name string
		b    func() interfaces.ReadOnlyBeaconBlock
		want string
	}{
		{name: "has attestation",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(&zondpb.BeaconBlockCapella{
					Body: &zondpb.BeaconBlockBodyCapella{
						Attestations:     []*zondpb.Attestation{{}},
						SyncAggregate:    &zondpb.SyncAggregate{},
						ExecutionPayload: &enginev1.ExecutionPayloadCapella{},
					}},
				)
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" attestations=1 payloadHash= prefix=blockchain slot=0 syncBitsCount=0 txCount=0",
		},
		{name: "has deposit",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(
					&zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{
						Attestations:     []*zondpb.Attestation{{}},
						Deposits:         []*zondpb.Deposit{{}},
						SyncAggregate:    &zondpb.SyncAggregate{},
						ExecutionPayload: &enginev1.ExecutionPayloadCapella{},
					}})
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" attestations=1 deposits=1 payloadHash= prefix=blockchain slot=0 syncBitsCount=0 txCount=0",
		},
		{name: "has attester slashing",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(&zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{
					AttesterSlashings: []*zondpb.AttesterSlashing{{}},
					SyncAggregate:     &zondpb.SyncAggregate{},
					ExecutionPayload:  &enginev1.ExecutionPayloadCapella{},
				}})
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" attesterSlashings=1 payloadHash= prefix=blockchain slot=0 syncBitsCount=0 txCount=0",
		},
		{name: "has proposer slashing",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(&zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{
					ProposerSlashings: []*zondpb.ProposerSlashing{{}},
					SyncAggregate:     &zondpb.SyncAggregate{},
					ExecutionPayload:  &enginev1.ExecutionPayloadCapella{},
				}})
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" payloadHash= prefix=blockchain proposerSlashings=1 slot=0 syncBitsCount=0 txCount=0",
		},
		{name: "has exit",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(&zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{
					VoluntaryExits:   []*zondpb.SignedVoluntaryExit{{}},
					SyncAggregate:    &zondpb.SyncAggregate{},
					ExecutionPayload: &enginev1.ExecutionPayloadCapella{},
				}})
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" payloadHash= prefix=blockchain slot=0 syncBitsCount=0 txCount=0 voluntaryExits=1",
		},
		{name: "has everything",
			b: func() interfaces.ReadOnlyBeaconBlock {
				wb, err := blocks.NewBeaconBlock(&zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{
					Attestations:      []*zondpb.Attestation{{}},
					Deposits:          []*zondpb.Deposit{{}},
					AttesterSlashings: []*zondpb.AttesterSlashing{{}},
					ProposerSlashings: []*zondpb.ProposerSlashing{{}},
					VoluntaryExits:    []*zondpb.SignedVoluntaryExit{{}},
					SyncAggregate:     &zondpb.SyncAggregate{},
					ExecutionPayload: &enginev1.ExecutionPayloadCapella{
						BlockHash:    []byte{1, 2, 3},
						Transactions: [][]byte{{}, {}},
					},
				}})
				require.NoError(t, err)
				return wb
			},
			want: "\"Finished applying state transition\" attestations=1 attesterSlashings=1 deposits=1 payloadHash=0x010203 prefix=blockchain proposerSlashings=1 slot=0 syncBitsCount=0 txCount=2 voluntaryExits=1",
		},
		{name: "has payload",
			b:    func() interfaces.ReadOnlyBeaconBlock { return wrappedPayloadBlk },
			want: "\"Finished applying state transition\" payloadHash=0x010203 prefix=blockchain slot=0 syncBitsCount=0 txCount=2",
		},
	}
	for _, tt := range tests {
		hook := logTest.NewGlobal()
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, logStateTransitionData(tt.b()))
			require.LogsContain(t, hook, tt.want)
		})
	}
}
