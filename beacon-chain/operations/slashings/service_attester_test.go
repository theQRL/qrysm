package slashings

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func validAttesterSlashingForValIdx(t *testing.T, beaconState state.BeaconState, privs []dilithium.DilithiumKey, valIdx ...uint64) *zondpb.AttesterSlashing {
	var slashings []*zondpb.AttesterSlashing
	for _, idx := range valIdx {
		slashing, err := util.GenerateAttesterSlashingForValidator(beaconState, privs[idx], primitives.ValidatorIndex(idx))
		require.NoError(t, err)
		slashings = append(slashings, slashing)
	}
	var allSigs1 [][]byte
	var allSigs2 [][]byte
	for _, slashing := range slashings {
		sigs1 := slashing.Attestation_1.Signatures
		sigs2 := slashing.Attestation_2.Signatures
		allSigs1 = append(allSigs1, sigs1...)
		allSigs2 = append(allSigs2, sigs2...)
	}
	aggSlashing := &zondpb.AttesterSlashing{
		Attestation_1: &zondpb.IndexedAttestation{
			AttestingIndices: valIdx,
			Data:             slashings[0].Attestation_1.Data,
			Signatures:       allSigs1,
		},
		Attestation_2: &zondpb.IndexedAttestation{
			AttestingIndices: valIdx,
			Data:             slashings[0].Attestation_2.Data,
			Signatures:       allSigs2,
		},
	}
	return aggSlashing
}

func attesterSlashingForValIdx(valIdx ...uint64) *zondpb.AttesterSlashing {
	return &zondpb.AttesterSlashing{
		Attestation_1: &zondpb.IndexedAttestation{AttestingIndices: valIdx},
		Attestation_2: &zondpb.IndexedAttestation{AttestingIndices: valIdx},
	}
}

func pendingSlashingForValIdx(valIdx ...uint64) *PendingAttesterSlashing {
	return &PendingAttesterSlashing{
		attesterSlashing: attesterSlashingForValIdx(valIdx...),
		validatorToSlash: primitives.ValidatorIndex(valIdx[0]),
	}
}

func TestPool_InsertAttesterSlashing(t *testing.T) {
	type fields struct {
		pending  []*PendingAttesterSlashing
		included map[primitives.ValidatorIndex]bool
		wantErr  []bool
	}
	type args struct {
		slashings []*zondpb.AttesterSlashing
	}

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)
	pendingSlashings := make([]*PendingAttesterSlashing, 20)
	slashings := make([]*zondpb.AttesterSlashing, 20)
	for i := 0; i < len(pendingSlashings); i++ {
		sl, err := util.GenerateAttesterSlashingForValidator(beaconState, privKeys[i], primitives.ValidatorIndex(i))
		require.NoError(t, err)
		pendingSlashings[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		slashings[i] = sl
	}
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	// We mark the following validators with some preconditions.
	exitedVal, err := beaconState.ValidatorAtIndex(primitives.ValidatorIndex(2))
	require.NoError(t, err)
	exitedVal.WithdrawableEpoch = 0
	require.NoError(t, beaconState.UpdateValidatorAtIndex(primitives.ValidatorIndex(2), exitedVal))
	futureWithdrawVal, err := beaconState.ValidatorAtIndex(primitives.ValidatorIndex(4))
	require.NoError(t, err)
	futureWithdrawVal.WithdrawableEpoch = 17
	require.NoError(t, beaconState.UpdateValidatorAtIndex(primitives.ValidatorIndex(4), futureWithdrawVal))
	slashedVal, err := beaconState.ValidatorAtIndex(primitives.ValidatorIndex(5))
	require.NoError(t, err)
	slashedVal.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(primitives.ValidatorIndex(5), slashedVal))
	slashedVal2, err := beaconState.ValidatorAtIndex(primitives.ValidatorIndex(21))
	require.NoError(t, err)
	slashedVal2.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(primitives.ValidatorIndex(21), slashedVal2))

	aggSlashing1 := validAttesterSlashingForValIdx(t, beaconState, privKeys, 0, 1, 2)
	aggSlashing2 := validAttesterSlashingForValIdx(t, beaconState, privKeys, 5, 9, 13)
	aggSlashing3 := validAttesterSlashingForValIdx(t, beaconState, privKeys, 15, 20, 21)
	aggSlashing4 := validAttesterSlashingForValIdx(t, beaconState, privKeys, 2, 5, 21)

	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*PendingAttesterSlashing
		err    string
	}{
		{
			name: "Empty list",
			fields: fields{
				pending:  make([]*PendingAttesterSlashing, 0),
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{false},
			},
			args: args{
				slashings: slashings[0:1],
			},
			want: []*PendingAttesterSlashing{
				{
					attesterSlashing: slashings[0],
					validatorToSlash: 0,
				},
			},
		},
		{
			name: "Empty list two validators slashed",
			fields: fields{
				pending:  make([]*PendingAttesterSlashing, 0),
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{false, false},
			},
			args: args{
				slashings: slashings[0:2],
			},
			want: pendingSlashings[0:2],
		},
		{
			name: "Duplicate identical slashing",
			fields: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashings[1],
				},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{true},
			},
			args: args{
				slashings: slashings[1:2],
			},
			want: pendingSlashings[1:2],
		},
		{
			name: "Slashing for already exit validator",
			fields: fields{
				pending:  []*PendingAttesterSlashing{},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{true},
			},
			args: args{
				slashings: slashings[5:6],
			},
			want: []*PendingAttesterSlashing{},
		},
		{
			name: "Slashing for withdrawable validator",
			fields: fields{
				pending:  []*PendingAttesterSlashing{},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{true},
			},
			args: args{
				slashings: slashings[2:3],
			},
			want: []*PendingAttesterSlashing{},
		},
		{
			name: "Slashing for slashed validator",
			fields: fields{
				pending:  []*PendingAttesterSlashing{},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{false},
			},
			args: args{
				slashings: slashings[4:5],
			},
			want: pendingSlashings[4:5],
		},
		{
			name: "Already included",
			fields: fields{
				pending: []*PendingAttesterSlashing{},
				included: map[primitives.ValidatorIndex]bool{
					1: true,
				},
				wantErr: []bool{true},
			},
			args: args{
				slashings: slashings[1:2],
			},
			want: []*PendingAttesterSlashing{},
		},
		{
			name: "Maintains sorted order",
			fields: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashings[0],
					pendingSlashings[2],
				},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{false},
			},
			args: args{
				slashings: slashings[1:2],
			},
			want: pendingSlashings[0:3],
		},
		{
			name: "Doesn't reject partially slashed slashings",
			fields: fields{
				pending:  []*PendingAttesterSlashing{},
				included: make(map[primitives.ValidatorIndex]bool),
				wantErr:  []bool{false, false, false, true},
			},
			args: args{
				slashings: []*zondpb.AttesterSlashing{
					aggSlashing1,
					aggSlashing2,
					aggSlashing3,
					aggSlashing4,
				},
			},
			want: []*PendingAttesterSlashing{
				{
					attesterSlashing: aggSlashing1,
					validatorToSlash: 0,
				},
				{
					attesterSlashing: aggSlashing1,
					validatorToSlash: 1,
				},
				{
					attesterSlashing: aggSlashing2,
					validatorToSlash: 9,
				},
				{
					attesterSlashing: aggSlashing2,
					validatorToSlash: 13,
				},
				{
					attesterSlashing: aggSlashing3,
					validatorToSlash: 15,
				},
				{
					attesterSlashing: aggSlashing3,
					validatorToSlash: 20,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pendingAttesterSlashing: tt.fields.pending,
				included:                tt.fields.included,
			}
			var err error
			for i := 0; i < len(tt.args.slashings); i++ {
				err = p.InsertAttesterSlashing(context.Background(), beaconState, tt.args.slashings[i])
				if tt.fields.wantErr[i] {
					assert.NotNil(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
			assert.Equal(t, len(tt.want), len(p.pendingAttesterSlashing))

			for i := range p.pendingAttesterSlashing {
				assert.Equal(t, tt.want[i].validatorToSlash, p.pendingAttesterSlashing[i].validatorToSlash)
				assert.DeepEqual(t, tt.want[i].attesterSlashing, p.pendingAttesterSlashing[i].attesterSlashing, "At index %d", i)
			}
		})
	}
}

func TestPool_InsertAttesterSlashing_SigFailsVerify_ClearPool(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.MaxAttesterSlashings = 2
	params.OverrideBeaconConfig(conf)
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)
	pendingSlashings := make([]*PendingAttesterSlashing, 2)
	slashings := make([]*zondpb.AttesterSlashing, 2)
	for i := 0; i < 2; i++ {
		sl, err := util.GenerateAttesterSlashingForValidator(beaconState, privKeys[i], primitives.ValidatorIndex(i))
		require.NoError(t, err)
		pendingSlashings[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		slashings[i] = sl
	}
	// We mess up the signature of the second slashing.
	badSig := make([]byte, 96)
	copy(badSig, "muahaha")
	pendingSlashings[1].attesterSlashing.Attestation_1.Signatures = [][]byte{badSig}
	slashings[1].Attestation_1.Signatures = [][]byte{badSig}
	p := &Pool{
		pendingAttesterSlashing: make([]*PendingAttesterSlashing, 0),
	}
	require.NoError(t, p.InsertAttesterSlashing(context.Background(), beaconState, slashings[0]))
	err := p.InsertAttesterSlashing(context.Background(), beaconState, slashings[1])
	require.ErrorContains(t, "could not verify attester slashing", err, "Expected error when inserting slashing with bad sig")
	assert.Equal(t, 1, len(p.pendingAttesterSlashing))
}

func TestPool_MarkIncludedAttesterSlashing(t *testing.T) {
	type fields struct {
		pending  []*PendingAttesterSlashing
		included map[primitives.ValidatorIndex]bool
	}
	type args struct {
		slashing *zondpb.AttesterSlashing
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   fields
	}{
		{
			name: "Included, does not exist in pending",
			fields: fields{
				pending: []*PendingAttesterSlashing{
					{
						attesterSlashing: attesterSlashingForValIdx(1),
						validatorToSlash: 1,
					},
				},
				included: make(map[primitives.ValidatorIndex]bool),
			},
			args: args{
				slashing: attesterSlashingForValIdx(3),
			},
			want: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashingForValIdx(1),
				},
				included: map[primitives.ValidatorIndex]bool{
					3: true,
				},
			},
		},
		{
			name: "Removes from pending list",
			fields: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashingForValIdx(1),
					pendingSlashingForValIdx(2),
					pendingSlashingForValIdx(3),
				},
				included: map[primitives.ValidatorIndex]bool{
					0: true,
				},
			},
			args: args{
				slashing: attesterSlashingForValIdx(2),
			},
			want: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashingForValIdx(1),
					pendingSlashingForValIdx(3),
				},
				included: map[primitives.ValidatorIndex]bool{
					0: true,
					2: true,
				},
			},
		},
		{
			name: "Removes from long pending list",
			fields: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashingForValIdx(1),
					pendingSlashingForValIdx(2),
					pendingSlashingForValIdx(3),
					pendingSlashingForValIdx(4),
					pendingSlashingForValIdx(5),
					pendingSlashingForValIdx(6),
					pendingSlashingForValIdx(7),
					pendingSlashingForValIdx(8),
					pendingSlashingForValIdx(9),
					pendingSlashingForValIdx(10),
					pendingSlashingForValIdx(11),
				},
				included: map[primitives.ValidatorIndex]bool{
					0: true,
				},
			},
			args: args{
				slashing: attesterSlashingForValIdx(6),
			},
			want: fields{
				pending: []*PendingAttesterSlashing{
					pendingSlashingForValIdx(1),
					pendingSlashingForValIdx(2),
					pendingSlashingForValIdx(3),
					pendingSlashingForValIdx(4),
					pendingSlashingForValIdx(5),
					pendingSlashingForValIdx(7),
					pendingSlashingForValIdx(8),
					pendingSlashingForValIdx(9),
					pendingSlashingForValIdx(10),
					pendingSlashingForValIdx(11),
				},
				included: map[primitives.ValidatorIndex]bool{
					0: true,
					6: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pendingAttesterSlashing: tt.fields.pending,
				included:                tt.fields.included,
			}
			p.MarkIncludedAttesterSlashing(tt.args.slashing)
			assert.Equal(t, len(tt.want.pending), len(p.pendingAttesterSlashing))
			for i := range p.pendingAttesterSlashing {
				assert.DeepEqual(t, tt.want.pending[i], p.pendingAttesterSlashing[i])
			}
			assert.DeepEqual(t, tt.want.included, p.included)
		})
	}
}

func TestPool_PendingAttesterSlashings(t *testing.T) {
	type fields struct {
		pending []*PendingAttesterSlashing
		all     bool
	}
	params.SetupTestConfigCleanup(t)
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)
	pendingSlashings := make([]*PendingAttesterSlashing, 20)
	slashings := make([]*zondpb.AttesterSlashing, 20)
	for i := 0; i < len(pendingSlashings); i++ {
		sl, err := util.GenerateAttesterSlashingForValidator(beaconState, privKeys[i], primitives.ValidatorIndex(i))
		require.NoError(t, err)
		pendingSlashings[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		slashings[i] = sl
	}
	tests := []struct {
		name   string
		fields fields
		want   []*zondpb.AttesterSlashing
	}{
		{
			name: "Empty list",
			fields: fields{
				pending: []*PendingAttesterSlashing{},
			},
			want: []*zondpb.AttesterSlashing{},
		},
		{
			name: "All pending",
			fields: fields{
				pending: pendingSlashings,
				all:     true,
			},
			want: slashings,
		},
		{
			name: "All eligible",
			fields: fields{
				pending: pendingSlashings,
			},
			want: slashings[0:2],
		},
		{
			name: "Multiple indices",
			fields: fields{
				pending: pendingSlashings[3:6],
			},
			want: slashings[3:5],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{
				pendingAttesterSlashing: tt.fields.pending,
			}
			assert.DeepEqual(t, tt.want, p.PendingAttesterSlashings(context.Background(), beaconState, tt.fields.all))
		})
	}
}

func TestPool_PendingAttesterSlashings_Slashed(t *testing.T) {
	type fields struct {
		pending []*PendingAttesterSlashing
		all     bool
	}
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.MaxAttesterSlashings = 2
	params.OverrideBeaconConfig(conf)
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)
	val, err := beaconState.ValidatorAtIndex(0)
	require.NoError(t, err)
	val.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(0, val))
	val, err = beaconState.ValidatorAtIndex(5)
	require.NoError(t, err)
	val.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(5, val))
	pendingSlashings := make([]*PendingAttesterSlashing, 20)
	pendingSlashings2 := make([]*PendingAttesterSlashing, 20)
	slashings := make([]*zondpb.AttesterSlashing, 20)
	for i := 0; i < len(pendingSlashings); i++ {
		sl, err := util.GenerateAttesterSlashingForValidator(beaconState, privKeys[i], primitives.ValidatorIndex(i))
		require.NoError(t, err)
		pendingSlashings[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		pendingSlashings2[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		slashings[i] = sl
	}
	result := append(slashings[1:5], slashings[6:]...)
	tests := []struct {
		name   string
		fields fields
		want   []*zondpb.AttesterSlashing
	}{
		{
			name: "One item",
			fields: fields{
				pending: pendingSlashings[:2],
			},
			want: slashings[1:2],
		},
		{
			name: "Skips gapped slashed",
			fields: fields{
				pending: pendingSlashings[4:7],
			},
			want: result[3:5],
		},
		{
			name: "All and skips gapped slashed validators",
			fields: fields{
				pending: pendingSlashings2,
				all:     true,
			},
			want: result,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pool{pendingAttesterSlashing: tt.fields.pending}
			assert.DeepEqual(t, tt.want, p.PendingAttesterSlashings(context.Background(), beaconState, tt.fields.all /*noLimit*/))
		})
	}
}

func TestPool_PendingAttesterSlashings_NoDuplicates(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.MaxAttesterSlashings = 2
	params.OverrideBeaconConfig(conf)
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)
	pendingSlashings := make([]*PendingAttesterSlashing, 3)
	slashings := make([]*zondpb.AttesterSlashing, 3)
	for i := 0; i < 2; i++ {
		sl, err := util.GenerateAttesterSlashingForValidator(beaconState, privKeys[i], primitives.ValidatorIndex(i))
		require.NoError(t, err)
		pendingSlashings[i] = &PendingAttesterSlashing{
			attesterSlashing: sl,
			validatorToSlash: primitives.ValidatorIndex(i),
		}
		slashings[i] = sl
	}
	// We duplicate the last slashing.
	pendingSlashings[2] = pendingSlashings[1]
	slashings[2] = slashings[1]
	p := &Pool{
		pendingAttesterSlashing: pendingSlashings,
	}
	assert.DeepEqual(t, slashings[0:2], p.PendingAttesterSlashings(context.Background(), beaconState, false /*noLimit*/))
}
