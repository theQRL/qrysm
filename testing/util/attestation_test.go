package util

import (
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	v1 "github.com/theQRL/qrysm/proto/zond/v1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestHydrateAttestation(t *testing.T) {
	a := HydrateAttestation(&zondpb.Attestation{})
	_, err := a.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, a.Signatures[0], make([]byte, fieldparams.DilithiumSignatureLength))
}

func TestHydrateAttestationData(t *testing.T) {
	d := HydrateAttestationData(&zondpb.AttestationData{})
	_, err := d.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, d.BeaconBlockRoot, make([]byte, fieldparams.RootLength))
	require.DeepEqual(t, d.Target.Root, make([]byte, fieldparams.RootLength))
	require.DeepEqual(t, d.Source.Root, make([]byte, fieldparams.RootLength))
}

func TestHydrateV1Attestation(t *testing.T) {
	a := HydrateV1Attestation(&v1.Attestation{})
	_, err := a.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, a.Signatures[0], make([]byte, fieldparams.DilithiumSignatureLength))
}

func TestHydrateV1AttestationData(t *testing.T) {
	d := HydrateV1AttestationData(&v1.AttestationData{})
	_, err := d.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, d.BeaconBlockRoot, make([]byte, fieldparams.RootLength))
	require.DeepEqual(t, d.Target.Root, make([]byte, fieldparams.RootLength))
	require.DeepEqual(t, d.Source.Root, make([]byte, fieldparams.RootLength))
}

func TestHydrateIndexedAttestation(t *testing.T) {
	a := &zondpb.IndexedAttestation{}
	a = HydrateIndexedAttestation(a)
	_, err := a.HashTreeRoot()
	require.NoError(t, err)
	_, err = a.Data.HashTreeRoot()
	require.NoError(t, err)
}

func TestGenerateAttestations_EpochBoundary(t *testing.T) {
	gs, pk := DeterministicGenesisStateCapella(t, 32)
	_, err := GenerateAttestations(gs, pk, 1, params.BeaconConfig().SlotsPerEpoch, false)
	require.NoError(t, err)
}
