package cache

import (
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSyncSubnetIDsCache_Roundtrip(t *testing.T) {
	c := newSyncSubnetIDs()

	for i := range 20 {
		pubkey := [field_params.MLDSA87PubkeyLength]byte{byte(i)}
		c.AddSyncCommitteeSubnets(pubkey[:], 100, []uint64{uint64(i)}, 0)
	}

	for i := range uint64(20) {
		pubkey := [field_params.MLDSA87PubkeyLength]byte{byte(i)}

		idxs, _, ok, _ := c.GetSyncCommitteeSubnets(pubkey[:], 100)
		if !ok {
			t.Errorf("Couldn't find entry in cache for pubkey %#x", pubkey)
			continue
		}
		require.Equal(t, i, idxs[0])
	}
	coms := c.GetAllSubnets(100)
	assert.Equal(t, 20, len(coms))
}

func TestSyncSubnetIDsCache_ValidateCurrentEpoch(t *testing.T) {
	c := newSyncSubnetIDs()

	for i := range 20 {
		pubkey := [field_params.MLDSA87PubkeyLength]byte{byte(i)}
		c.AddSyncCommitteeSubnets(pubkey[:], 100, []uint64{uint64(i)}, 0)
	}

	coms := c.GetAllSubnets(50)
	assert.Equal(t, 0, len(coms))

	for i := range uint64(20) {
		pubkey := [field_params.MLDSA87PubkeyLength]byte{byte(i)}

		_, jEpoch, ok, _ := c.GetSyncCommitteeSubnets(pubkey[:], 100)
		if !ok {
			t.Errorf("Couldn't find entry in cache for pubkey %#x", pubkey)
			continue
		}
		require.Equal(t, true, uint64(jEpoch) >= 100-params.BeaconConfig().SyncCommitteeSubnetCount)
	}

	coms = c.GetAllSubnets(99)
	assert.Equal(t, 20, len(coms))
}
