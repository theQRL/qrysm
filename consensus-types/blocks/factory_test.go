package blocks

import (
	"bytes"
	"errors"
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func Test_NewSignedBeaconBlock(t *testing.T) {
	t.Run("GenericSignedBeaconBlock_Zond", func(t *testing.T) {
		pb := &qrysmpb.GenericSignedBeaconBlock_Zond{
			Zond: &qrysmpb.SignedBeaconBlockZond{
				Block: &qrysmpb.BeaconBlockZond{
					Body: &qrysmpb.BeaconBlockBodyZond{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
	})
	t.Run("SignedBeaconBlockZond", func(t *testing.T) {
		pb := &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
	})
	t.Run("GenericSignedBeaconBlock_BlindedZond", func(t *testing.T) {
		pb := &qrysmpb.GenericSignedBeaconBlock_BlindedZond{
			BlindedZond: &qrysmpb.SignedBlindedBeaconBlockZond{
				Block: &qrysmpb.BlindedBeaconBlockZond{
					Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("SignedBlindedBeaconBlockZond", func(t *testing.T) {
		pb := &qrysmpb.SignedBlindedBeaconBlockZond{
			Block: &qrysmpb.BlindedBeaconBlockZond{
				Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("nil", func(t *testing.T) {
		_, err := NewSignedBeaconBlock(nil)
		assert.ErrorContains(t, "received nil object", err)
	})
	t.Run("unsupported type", func(t *testing.T) {
		_, err := NewSignedBeaconBlock(&bytes.Reader{})
		assert.ErrorContains(t, "unable to create block from type *bytes.Reader", err)
	})
}

func Test_NewBeaconBlock(t *testing.T) {
	t.Run("GenericBeaconBlock_Zond", func(t *testing.T) {
		pb := &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Body: &qrysmpb.BeaconBlockBodyZond{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
	})
	t.Run("BeaconBlockZond", func(t *testing.T) {
		pb := &qrysmpb.BeaconBlockZond{Body: &qrysmpb.BeaconBlockBodyZond{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
	})
	t.Run("GenericBeaconBlock_BlindedZond", func(t *testing.T) {
		pb := &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("BlindedBeaconBlockZond", func(t *testing.T) {
		pb := &qrysmpb.BlindedBeaconBlockZond{Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Zond, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("nil", func(t *testing.T) {
		_, err := NewBeaconBlock(nil)
		assert.ErrorContains(t, "received nil object", err)
	})
	t.Run("unsupported type", func(t *testing.T) {
		_, err := NewBeaconBlock(&bytes.Reader{})
		assert.ErrorContains(t, "unable to create block from type *bytes.Reader", err)
	})
}

func Test_NewBeaconBlockBody(t *testing.T) {
	t.Run("BeaconBlockBodyZond", func(t *testing.T) {
		pb := &qrysmpb.BeaconBlockBodyZond{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Zond, b.version)
	})
	t.Run("BlindedBeaconBlockBodyZond", func(t *testing.T) {
		pb := &qrysmpb.BlindedBeaconBlockBodyZond{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Zond, b.version)
		assert.Equal(t, true, b.isBlinded)
	})

	t.Run("nil", func(t *testing.T) {
		_, err := NewBeaconBlockBody(nil)
		assert.ErrorContains(t, "received nil object", err)
	})
	t.Run("unsupported type", func(t *testing.T) {
		_, err := NewBeaconBlockBody(&bytes.Reader{})
		assert.ErrorContains(t, "unable to create block body from type *bytes.Reader", err)
	})
}

func Test_BuildSignedBeaconBlock(t *testing.T) {
	sig := bytesutil.ToBytes4627([]byte("signature"))
	t.Run("Zond", func(t *testing.T) {
		b := &BeaconBlock{version: version.Zond, body: &BeaconBlockBody{version: version.Zond}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Zond, sb.Version())
	})
	t.Run("ZondBlind", func(t *testing.T) {
		b := &BeaconBlock{version: version.Zond, body: &BeaconBlockBody{version: version.Zond, isBlinded: true}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Zond, sb.Version())
		assert.Equal(t, true, sb.IsBlinded())
	})
}

func TestBuildSignedBeaconBlockFromExecutionPayload(t *testing.T) {
	t.Run("nil block check", func(t *testing.T) {
		_, err := BuildSignedBeaconBlockFromExecutionPayload(nil, nil)
		require.ErrorIs(t, ErrNilSignedBeaconBlock, err)
	})

	t.Run("not blinded payload", func(t *testing.T) {
		zondBlock := &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Body: &qrysmpb.BeaconBlockBodyZond{}}}
		blk, err := NewSignedBeaconBlock(zondBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, nil)
		require.Equal(t, true, errors.Is(err, errNonBlindedSignedBeaconBlock))
	})
	t.Run("payload header root and payload root mismatch", func(t *testing.T) {
		blockHash := bytesutil.Bytes32(1)
		payload := &enginev1.ExecutionPayloadZond{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     blockHash,
			Transactions:  make([][]byte, 0),
		}
		wrapped, err := WrappedExecutionPayloadZond(payload, 0)
		require.NoError(t, err)
		header, err := PayloadToHeaderZond(wrapped)
		require.NoError(t, err)
		blindedBlock := &qrysmpb.SignedBlindedBeaconBlockZond{
			Block: &qrysmpb.BlindedBeaconBlockZond{
				Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}}

		// Modify the header.
		header.GasUsed += 1
		blindedBlock.Block.Body.ExecutionPayloadHeader = header

		blk, err := NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, payload)
		require.ErrorContains(t, "roots do not match", err)
	})
	t.Run("ok", func(t *testing.T) {
		payload := &enginev1.ExecutionPayloadZond{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
		}
		wrapped, err := WrappedExecutionPayloadZond(payload, 0)
		require.NoError(t, err)
		header, err := PayloadToHeaderZond(wrapped)
		require.NoError(t, err)
		blindedBlock := &qrysmpb.SignedBlindedBeaconBlockZond{
			Block: &qrysmpb.BlindedBeaconBlockZond{
				Body: &qrysmpb.BlindedBeaconBlockBodyZond{}}}
		blindedBlock.Block.Body.ExecutionPayloadHeader = header

		blk, err := NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		builtBlock, err := BuildSignedBeaconBlockFromExecutionPayload(blk, payload)
		require.NoError(t, err)

		got, err := builtBlock.Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())
	})
}
