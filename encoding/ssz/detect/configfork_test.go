package detect

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestSlotFromBlock(t *testing.T) {
	b := util.NewBeaconBlockCapella()
	var slot primitives.Slot = 3
	b.Block.Slot = slot
	bb, err := b.MarshalSSZ()
	require.NoError(t, err)
	sfb, err := slotFromBlock(bb)
	require.NoError(t, err)
	require.Equal(t, slot, sfb)
}

func TestByState(t *testing.T) {
	bc := params.BeaconConfig()
	cases := []struct {
		name        string
		version     int
		slot        primitives.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Capella,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		require.NoError(t, st.SetFork(&zondpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  c.forkversion[:],
			Epoch:           0,
		}))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		require.Equal(t, c.version, cf.Fork)
		require.Equal(t, c.forkversion, cf.Version)
		require.Equal(t, bc.ConfigName, cf.Config.ConfigName)
	}
}

func stateForVersion(v int) (state.BeaconState, error) {
	switch v {
	case version.Capella:
		return util.NewBeaconStateCapella()
	default:
		return nil, fmt.Errorf("unrecognized version %d", v)
	}
}

func TestUnmarshalState(t *testing.T) {
	ctx := context.Background()

	bc := params.BeaconConfig()

	cases := []struct {
		name        string
		version     int
		slot        primitives.Slot
		forkversion [4]byte
	}{
		{
			name:        "genesis",
			version:     version.Capella,
			slot:        0,
			forkversion: bytesutil.ToBytes4(bc.GenesisForkVersion),
		},
	}
	for _, c := range cases {
		st, err := stateForVersion(c.version)
		require.NoError(t, err)
		require.NoError(t, st.SetFork(&zondpb.Fork{
			PreviousVersion: make([]byte, 4),
			CurrentVersion:  c.forkversion[:],
			Epoch:           0,
		}))
		require.NoError(t, st.SetSlot(c.slot))
		m, err := st.MarshalSSZ()
		require.NoError(t, err)
		cf, err := FromState(m)
		require.NoError(t, err)
		s, err := cf.UnmarshalBeaconState(m)
		require.NoError(t, err)
		expected, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		actual, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, expected, actual)
	}
}

func TestUnmarshalBlock(t *testing.T) {
	genv := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)

	cases := []struct {
		b       func(*testing.T, primitives.Slot) interfaces.ReadOnlySignedBeaconBlock
		name    string
		version [4]byte
		slot    primitives.Slot
		err     error
	}{
		{
			name:    "genesis - slot 0",
			b:       signedTestBlockCapella,
			version: genv,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromForkVersion(c.version)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBeaconBlock(marshaled)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestUnmarshalBlindedBlock(t *testing.T) {
	genv := bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)
	cases := []struct {
		b       func(*testing.T, primitives.Slot) interfaces.ReadOnlySignedBeaconBlock
		name    string
		version [4]byte
		slot    primitives.Slot
		err     error
	}{
		{
			name:    "genesis - slot 0",
			b:       signedTestBlindedBlockCapella,
			version: genv,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.b(t, c.slot)
			marshaled, err := b.MarshalSSZ()
			require.NoError(t, err)
			cf, err := FromForkVersion(c.version)
			require.NoError(t, err)
			bcf, err := cf.UnmarshalBlindedBeaconBlock(marshaled)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			expected, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			actual, err := bcf.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func signedTestBlockCapella(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockCapella()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}

func signedTestBlindedBlockCapella(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBlindedBeaconBlockCapella()
	b.Block.Slot = slot
	s, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return s
}
