package cache

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSyncCommitteeHeadState(t *testing.T) {
	capellaState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Fork: &zondpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	type put struct {
		slot  primitives.Slot
		state state.BeaconState
	}
	tests := []struct {
		name       string
		key        primitives.Slot
		put        *put
		want       state.BeaconState
		wantErr    bool
		wantPutErr bool
	}{
		{
			name: "putting error in",
			key:  primitives.Slot(1),
			put: &put{
				slot:  primitives.Slot(1),
				state: nil,
			},
			wantPutErr: true,
			wantErr:    true,
		},
		{
			name:    "not found when empty cache",
			key:     primitives.Slot(1),
			wantErr: true,
		},
		{
			name: "not found when non-existent key in non-empty cache",
			key:  primitives.Slot(2),
			put: &put{
				slot:  primitives.Slot(1),
				state: capellaState,
			},
			wantErr: true,
		},
		{
			name: "found with key",
			key:  primitives.Slot(1),
			put: &put{
				slot:  primitives.Slot(1),
				state: capellaState,
			},
			want: capellaState,
		},
		{
			name: "not found when non-existent key in non-empty cache (bellatrix state)",
			key:  primitives.Slot(2),
			put: &put{
				slot:  primitives.Slot(1),
				state: capellaState,
			},
			wantErr: true,
		},
		{
			name: "found with key (capella state)",
			key:  primitives.Slot(200),
			put: &put{
				slot:  primitives.Slot(200),
				state: capellaState,
			},
			want: capellaState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSyncCommitteeHeadState()
			if tt.put != nil {
				err := c.Put(tt.put.slot, tt.put.state)
				if (err != nil) != tt.wantPutErr {
					t.Fatalf("Put() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
			got, err := c.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.DeepEqual(t, tt.want, got)
		})
	}
}
