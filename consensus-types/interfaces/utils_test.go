package interfaces_test

import (
	"testing"

	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestBeaconBlockHeaderFromBlock(t *testing.T) {
	hashLen := 32
	blk := &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBody{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), dilithium2.CryptoBytes),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
		},
	}
	bodyRoot, err := blk.Body.HashTreeRoot()
	require.NoError(t, err)
	want := &zond.BeaconBlockHeader{
		Slot:          blk.Slot,
		ProposerIndex: blk.ProposerIndex,
		ParentRoot:    blk.ParentRoot,
		StateRoot:     blk.StateRoot,
		BodyRoot:      bodyRoot[:],
	}

	bh, err := interfaces.BeaconBlockHeaderFromBlock(blk)
	require.NoError(t, err)
	assert.DeepEqual(t, want, bh)
}

func TestBeaconBlockHeaderFromBlockInterface(t *testing.T) {
	hashLen := 32
	blk := &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBody{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), dilithium2.CryptoBytes),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
		},
	}
	bodyRoot, err := blk.Body.HashTreeRoot()
	require.NoError(t, err)
	want := &zond.BeaconBlockHeader{
		Slot:          blk.Slot,
		ProposerIndex: blk.ProposerIndex,
		ParentRoot:    blk.ParentRoot,
		StateRoot:     blk.StateRoot,
		BodyRoot:      bodyRoot[:],
	}

	wb, err := blocks.NewBeaconBlock(blk)
	require.NoError(t, err)
	bh, err := interfaces.BeaconBlockHeaderFromBlockInterface(wb)
	require.NoError(t, err)
	assert.DeepEqual(t, want, bh)
}

func TestBeaconBlockHeaderFromBlock_NilBlockBody(t *testing.T) {
	hashLen := 32
	blk := &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
	}
	_, err := interfaces.BeaconBlockHeaderFromBlock(blk)
	require.ErrorContains(t, "nil block body", err)
}

func TestSignedBeaconBlockHeaderFromBlock(t *testing.T) {
	hashLen := 32
	blk := &zond.SignedBeaconBlock{Block: &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBody{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), dilithium2.CryptoBytes),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
		},
	},
		Signature: bytesutil.PadTo([]byte("signature"), dilithium2.CryptoBytes),
	}
	bodyRoot, err := blk.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	want := &zond.SignedBeaconBlockHeader{Header: &zond.BeaconBlockHeader{
		Slot:          blk.Block.Slot,
		ProposerIndex: blk.Block.ProposerIndex,
		ParentRoot:    blk.Block.ParentRoot,
		StateRoot:     blk.Block.StateRoot,
		BodyRoot:      bodyRoot[:],
	},
		Signature: blk.Signature,
	}

	bh, err := interfaces.SignedBeaconBlockHeaderFromBlock(blk)
	require.NoError(t, err)
	assert.DeepEqual(t, want, bh)
}

func TestSignedBeaconBlockHeaderFromBlockInterface(t *testing.T) {
	hashLen := 32
	blk := &zond.SignedBeaconBlock{Block: &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBody{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), dilithium2.CryptoBytes),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
		},
	},
		Signature: bytesutil.PadTo([]byte("signature"), dilithium2.CryptoBytes),
	}
	bodyRoot, err := blk.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	want := &zond.SignedBeaconBlockHeader{Header: &zond.BeaconBlockHeader{
		Slot:          blk.Block.Slot,
		ProposerIndex: blk.Block.ProposerIndex,
		ParentRoot:    blk.Block.ParentRoot,
		StateRoot:     blk.Block.StateRoot,
		BodyRoot:      bodyRoot[:],
	},
		Signature: blk.Signature,
	}
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	bh, err := interfaces.SignedBeaconBlockHeaderFromBlockInterface(wsb)
	require.NoError(t, err)
	assert.DeepEqual(t, want, bh)
}

func TestSignedBeaconBlockHeaderFromBlock_NilBlockBody(t *testing.T) {
	hashLen := 32
	blk := &zond.SignedBeaconBlock{Block: &zond.BeaconBlock{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
	},
		Signature: bytesutil.PadTo([]byte("signature"), dilithium2.CryptoBytes),
	}
	_, err := interfaces.SignedBeaconBlockHeaderFromBlock(blk)
	require.ErrorContains(t, "nil block", err)
}
