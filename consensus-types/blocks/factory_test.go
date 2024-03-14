package blocks

import (
	"bytes"
	"errors"
	"testing"

	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_NewSignedBeaconBlock(t *testing.T) {
	t.Run("GenericSignedBeaconBlock_Capella", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_Capella{
			Capella: &zond.SignedBeaconBlockCapella{
				Block: &zond.BeaconBlockCapella{
					Body: &zond.BeaconBlockBodyCapella{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
	})
	t.Run("SignedBeaconBlockCapella", func(t *testing.T) {
		pb := &zond.SignedBeaconBlockCapella{
			Block: &zond.BeaconBlockCapella{
				Body: &zond.BeaconBlockBodyCapella{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
	})
	t.Run("GenericSignedBeaconBlock_BlindedCapella", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_BlindedCapella{
			BlindedCapella: &zond.SignedBlindedBeaconBlockCapella{
				Block: &zond.BlindedBeaconBlockCapella{
					Body: &zond.BlindedBeaconBlockBodyCapella{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("SignedBlindedBeaconBlockCapella", func(t *testing.T) {
		pb := &zond.SignedBlindedBeaconBlockCapella{
			Block: &zond.BlindedBeaconBlockCapella{
				Body: &zond.BlindedBeaconBlockBodyCapella{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
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
	t.Run("GenericBeaconBlock_Capella", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_Capella{Capella: &zond.BeaconBlockCapella{Body: &zond.BeaconBlockBodyCapella{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
	})
	t.Run("BeaconBlockCapella", func(t *testing.T) {
		pb := &zond.BeaconBlockCapella{Body: &zond.BeaconBlockBodyCapella{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
	})
	t.Run("GenericBeaconBlock_BlindedCapella", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zond.BlindedBeaconBlockCapella{Body: &zond.BlindedBeaconBlockBodyCapella{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("BlindedBeaconBlockCapella", func(t *testing.T) {
		pb := &zond.BlindedBeaconBlockCapella{Body: &zond.BlindedBeaconBlockBodyCapella{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Capella, b.Version())
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
	t.Run("BeaconBlockBodyCapella", func(t *testing.T) {
		pb := &zond.BeaconBlockBodyCapella{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Capella, b.version)
	})
	t.Run("BlindedBeaconBlockBodyCapella", func(t *testing.T) {
		pb := &zond.BlindedBeaconBlockBodyCapella{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Capella, b.version)
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
	sig := bytesutil.ToBytes4595([]byte("signature"))
	t.Run("Capella", func(t *testing.T) {
		b := &BeaconBlock{version: version.Capella, body: &BeaconBlockBody{version: version.Capella}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Capella, sb.Version())
	})
	t.Run("CapellaBlind", func(t *testing.T) {
		b := &BeaconBlock{version: version.Capella, body: &BeaconBlockBody{version: version.Capella, isBlinded: true}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Capella, sb.Version())
		assert.Equal(t, true, sb.IsBlinded())
	})
}

func TestBuildSignedBeaconBlockFromExecutionPayload(t *testing.T) {
	t.Run("nil block check", func(t *testing.T) {
		_, err := BuildSignedBeaconBlockFromExecutionPayload(nil, nil)
		require.ErrorIs(t, ErrNilSignedBeaconBlock, err)
	})

	t.Run("not blinded payload", func(t *testing.T) {
		capellaBlock := &zond.SignedBeaconBlockCapella{
			Block: &zond.BeaconBlockCapella{
				Body: &zond.BeaconBlockBodyCapella{}}}
		blk, err := NewSignedBeaconBlock(capellaBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, nil)
		require.Equal(t, true, errors.Is(err, errNonBlindedSignedBeaconBlock))
	})
	t.Run("payload header root and payload root mismatch", func(t *testing.T) {
		blockHash := bytesutil.Bytes32(1)
		payload := &enginev1.ExecutionPayloadCapella{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     blockHash,
			Transactions:  make([][]byte, 0),
		}
		wrapped, err := WrappedExecutionPayloadCapella(payload, 0)
		require.NoError(t, err)
		header, err := PayloadToHeaderCapella(wrapped)
		require.NoError(t, err)
		blindedBlock := &zond.SignedBlindedBeaconBlockCapella{
			Block: &zond.BlindedBeaconBlockCapella{
				Body: &zond.BlindedBeaconBlockBodyCapella{}}}

		// Modify the header.
		header.GasUsed += 1
		blindedBlock.Block.Body.ExecutionPayloadHeader = header

		blk, err := NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, payload)
		require.ErrorContains(t, "roots do not match", err)
	})
	t.Run("ok", func(t *testing.T) {
		payload := &enginev1.ExecutionPayloadCapella{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
		}
		wrapped, err := WrappedExecutionPayloadCapella(payload, 0)
		require.NoError(t, err)
		header, err := PayloadToHeaderCapella(wrapped)
		require.NoError(t, err)
		blindedBlock := &zond.SignedBlindedBeaconBlockCapella{
			Block: &zond.BlindedBeaconBlockCapella{
				Body: &zond.BlindedBeaconBlockBodyCapella{}}}
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
