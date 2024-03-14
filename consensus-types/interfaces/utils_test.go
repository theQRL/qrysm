package interfaces_test

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestBeaconBlockHeaderFromBlock(t *testing.T) {
	hashLen := 32
	blk := &zond.BeaconBlockCapella{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBodyCapella{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), field_params.DilithiumSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
			SyncAggregate: &zond.SyncAggregate{
				SyncCommitteeBits:       []byte("sb"),
				SyncCommitteeSignatures: [][]byte{},
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				ParentHash:    bytesutil.PadTo([]byte("parent root"), hashLen),
				FeeRecipient:  bytesutil.PadTo([]byte("fee recipient"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts root"), hashLen),
				LogsBloom:     bytesutil.PadTo([]byte("state root"), 256),
				PrevRandao:    bytesutil.PadTo([]byte("prev randao"), hashLen),
				ExtraData:     bytesutil.PadTo([]byte("extra data"), hashLen),
				BaseFeePerGas: bytesutil.PadTo([]byte("base fee per gas"), hashLen),
				BlockHash:     bytesutil.PadTo([]byte("block hash"), hashLen),
				Transactions:  make([][]byte, 0),
				Withdrawals:   make([]*enginev1.Withdrawal, 0),
			},
			DilithiumToExecutionChanges: []*zond.SignedDilithiumToExecutionChange{},
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
	blk := &zond.BeaconBlockCapella{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBodyCapella{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), field_params.DilithiumSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
			SyncAggregate: &zond.SyncAggregate{
				SyncCommitteeBits:       []byte("sb"),
				SyncCommitteeSignatures: [][]byte{},
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				ParentHash:    bytesutil.PadTo([]byte("parent root"), hashLen),
				FeeRecipient:  bytesutil.PadTo([]byte("fee recipient"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts root"), hashLen),
				LogsBloom:     bytesutil.PadTo([]byte("state root"), 256),
				PrevRandao:    bytesutil.PadTo([]byte("prev randao"), hashLen),
				ExtraData:     bytesutil.PadTo([]byte("extra data"), hashLen),
				BaseFeePerGas: bytesutil.PadTo([]byte("base fee per gas"), hashLen),
				BlockHash:     bytesutil.PadTo([]byte("block hash"), hashLen),
				Transactions:  make([][]byte, 0),
				Withdrawals:   make([]*enginev1.Withdrawal, 0),
			},
			DilithiumToExecutionChanges: []*zond.SignedDilithiumToExecutionChange{},
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
	blk := &zond.BeaconBlockCapella{
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
	blk := &zond.SignedBeaconBlockCapella{Block: &zond.BeaconBlockCapella{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBodyCapella{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), field_params.DilithiumSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
			SyncAggregate: &zond.SyncAggregate{
				SyncCommitteeBits: bitfield.NewBitvector16(),
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				ParentHash:    bytesutil.PadTo([]byte("parent root"), hashLen),
				FeeRecipient:  bytesutil.PadTo([]byte("fee recipient"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts root"), hashLen),
				LogsBloom:     bytesutil.PadTo([]byte("state root"), 256),
				PrevRandao:    bytesutil.PadTo([]byte("prev randao"), hashLen),
				ExtraData:     bytesutil.PadTo([]byte("extra data"), hashLen),
				BaseFeePerGas: bytesutil.PadTo([]byte("base fee per gas"), hashLen),
				BlockHash:     bytesutil.PadTo([]byte("block hash"), hashLen),
				Transactions:  make([][]byte, 0),
				Withdrawals:   make([]*enginev1.Withdrawal, 0),
			},
			DilithiumToExecutionChanges: []*zond.SignedDilithiumToExecutionChange{},
		},
	},
		Signature: bytesutil.PadTo([]byte("signature"), field_params.DilithiumSignatureLength),
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
	blk := &zond.SignedBeaconBlockCapella{Block: &zond.BeaconBlockCapella{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
		Body: &zond.BeaconBlockBodyCapella{
			Eth1Data: &zond.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("randao"), field_params.DilithiumSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*zond.ProposerSlashing{},
			AttesterSlashings: []*zond.AttesterSlashing{},
			Attestations:      []*zond.Attestation{},
			Deposits:          []*zond.Deposit{},
			VoluntaryExits:    []*zond.SignedVoluntaryExit{},
			SyncAggregate: &zond.SyncAggregate{
				SyncCommitteeBits:       []byte("sb"),
				SyncCommitteeSignatures: [][]byte{},
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				ParentHash:    bytesutil.PadTo([]byte("parent root"), hashLen),
				FeeRecipient:  bytesutil.PadTo([]byte("fee recipient"), 20),
				StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
				ReceiptsRoot:  bytesutil.PadTo([]byte("receipts root"), hashLen),
				LogsBloom:     bytesutil.PadTo([]byte("state root"), 256),
				PrevRandao:    bytesutil.PadTo([]byte("prev randao"), hashLen),
				ExtraData:     bytesutil.PadTo([]byte("extra data"), hashLen),
				BaseFeePerGas: bytesutil.PadTo([]byte("base fee per gas"), hashLen),
				BlockHash:     bytesutil.PadTo([]byte("block hash"), hashLen),
				Transactions:  make([][]byte, 0),
				Withdrawals:   make([]*enginev1.Withdrawal, 0),
			},
			DilithiumToExecutionChanges: []*zond.SignedDilithiumToExecutionChange{},
		},
	},
		Signature: bytesutil.PadTo([]byte("signature"), field_params.DilithiumSignatureLength),
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
	blk := &zond.SignedBeaconBlockCapella{Block: &zond.BeaconBlockCapella{
		Slot:          200,
		ProposerIndex: 2,
		ParentRoot:    bytesutil.PadTo([]byte("parent root"), hashLen),
		StateRoot:     bytesutil.PadTo([]byte("state root"), hashLen),
	},
		Signature: bytesutil.PadTo([]byte("signature"), field_params.DilithiumSignatureLength),
	}
	_, err := interfaces.SignedBeaconBlockHeaderFromBlock(blk)
	require.ErrorContains(t, "nil block", err)
}
