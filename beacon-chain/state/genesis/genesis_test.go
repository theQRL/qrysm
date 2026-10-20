package genesis_test

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state/genesis"
	"github.com/theQRL/qrysm/config/params"
)

func TestGenesisState(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: params.MainnetName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := genesis.State(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			if st == nil {
				t.Fatal("nil state")
			}
			if st.NumValidators() <= 0 {
				t.Error("No validators present in state")
			}
			if got, want := len(st.Slashings()), int(params.MainnetConfig().EpochsPerSlashingsVector); got != want {
				t.Errorf("embedded genesis slashings length = %d, want %d", got, want)
			}
			for i, amount := range st.Slashings() {
				if amount != 0 {
					t.Errorf("embedded genesis slashings[%d] = %d, want zero", i, amount)
				}
			}
			if _, err := st.MarshalSSZ(); err != nil {
				t.Fatalf("embedded genesis does not serialize with the compiled layout: %v", err)
			}
		})
	}
}
