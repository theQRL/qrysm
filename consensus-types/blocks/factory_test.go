package blocks

import (
	"bytes"
	"errors"
	"testing"

	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_NewSignedBeaconBlock(t *testing.T) {
	t.Run("GenericSignedBeaconBlock_Phase0", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_Phase0{
			Phase0: &zond.SignedBeaconBlock{
				Block: &zond.BeaconBlock{
					Body: &zond.BeaconBlockBody{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Phase0, b.Version())
	})
	t.Run("SignedBeaconBlock", func(t *testing.T) {
		pb := &zond.SignedBeaconBlock{
			Block: &zond.BeaconBlock{
				Body: &zond.BeaconBlockBody{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Phase0, b.Version())
	})
	t.Run("GenericSignedBeaconBlock_Altair", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_Altair{
			Altair: &zond.SignedBeaconBlockAltair{
				Block: &zond.BeaconBlockAltair{
					Body: &zond.BeaconBlockBodyAltair{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Altair, b.Version())
	})
	t.Run("SignedBeaconBlockAltair", func(t *testing.T) {
		pb := &zond.SignedBeaconBlockAltair{
			Block: &zond.BeaconBlockAltair{
				Body: &zond.BeaconBlockBodyAltair{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Altair, b.Version())
	})
	t.Run("GenericSignedBeaconBlock_Bellatrix", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_Bellatrix{
			Bellatrix: &zond.SignedBeaconBlockBellatrix{
				Block: &zond.BeaconBlockBellatrix{
					Body: &zond.BeaconBlockBodyBellatrix{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
	})
	t.Run("SignedBeaconBlockBellatrix", func(t *testing.T) {
		pb := &zond.SignedBeaconBlockBellatrix{
			Block: &zond.BeaconBlockBellatrix{
				Body: &zond.BeaconBlockBodyBellatrix{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
	})
	t.Run("GenericSignedBeaconBlock_BlindedBellatrix", func(t *testing.T) {
		pb := &zond.GenericSignedBeaconBlock_BlindedBellatrix{
			BlindedBellatrix: &zond.SignedBlindedBeaconBlockBellatrix{
				Block: &zond.BlindedBeaconBlockBellatrix{
					Body: &zond.BlindedBeaconBlockBodyBellatrix{}}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("SignedBlindedBeaconBlockBellatrix", func(t *testing.T) {
		pb := &zond.SignedBlindedBeaconBlockBellatrix{
			Block: &zond.BlindedBeaconBlockBellatrix{
				Body: &zond.BlindedBeaconBlockBodyBellatrix{}}}
		b, err := NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
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
	t.Run("GenericBeaconBlock_Phase0", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_Phase0{Phase0: &zond.BeaconBlock{Body: &zond.BeaconBlockBody{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Phase0, b.Version())
	})
	t.Run("BeaconBlock", func(t *testing.T) {
		pb := &zond.BeaconBlock{Body: &zond.BeaconBlockBody{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Phase0, b.Version())
	})
	t.Run("GenericBeaconBlock_Altair", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_Altair{Altair: &zond.BeaconBlockAltair{Body: &zond.BeaconBlockBodyAltair{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Altair, b.Version())
	})
	t.Run("BeaconBlockAltair", func(t *testing.T) {
		pb := &zond.BeaconBlockAltair{Body: &zond.BeaconBlockBodyAltair{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Altair, b.Version())
	})
	t.Run("GenericBeaconBlock_Bellatrix", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_Bellatrix{Bellatrix: &zond.BeaconBlockBellatrix{Body: &zond.BeaconBlockBodyBellatrix{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
	})
	t.Run("BeaconBlockBellatrix", func(t *testing.T) {
		pb := &zond.BeaconBlockBellatrix{Body: &zond.BeaconBlockBodyBellatrix{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
	})
	t.Run("GenericBeaconBlock_BlindedBellatrix", func(t *testing.T) {
		pb := &zond.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zond.BlindedBeaconBlockBellatrix{Body: &zond.BlindedBeaconBlockBodyBellatrix{}}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
	t.Run("BlindedBeaconBlockBellatrix", func(t *testing.T) {
		pb := &zond.BlindedBeaconBlockBellatrix{Body: &zond.BlindedBeaconBlockBodyBellatrix{}}
		b, err := NewBeaconBlock(pb)
		require.NoError(t, err)
		assert.Equal(t, version.Bellatrix, b.Version())
		assert.Equal(t, true, b.IsBlinded())
	})
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
	t.Run("BeaconBlockBody", func(t *testing.T) {
		pb := &zond.BeaconBlockBody{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Phase0, b.version)
	})
	t.Run("BeaconBlockBodyAltair", func(t *testing.T) {
		pb := &zond.BeaconBlockBodyAltair{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Altair, b.version)
	})
	t.Run("BeaconBlockBodyBellatrix", func(t *testing.T) {
		pb := &zond.BeaconBlockBodyBellatrix{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Bellatrix, b.version)
	})
	t.Run("BlindedBeaconBlockBodyBellatrix", func(t *testing.T) {
		pb := &zond.BlindedBeaconBlockBodyBellatrix{}
		i, err := NewBeaconBlockBody(pb)
		require.NoError(t, err)
		b, ok := i.(*BeaconBlockBody)
		require.Equal(t, true, ok)
		assert.Equal(t, version.Bellatrix, b.version)
		assert.Equal(t, true, b.isBlinded)
	})
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
	sig := bytesutil.ToBytes96([]byte("signature"))
	t.Run("Phase0", func(t *testing.T) {
		b := &BeaconBlock{version: version.Phase0, body: &BeaconBlockBody{version: version.Phase0}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Phase0, sb.Version())
	})
	t.Run("Altair", func(t *testing.T) {
		b := &BeaconBlock{version: version.Altair, body: &BeaconBlockBody{version: version.Altair}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Altair, sb.Version())
	})
	t.Run("Bellatrix", func(t *testing.T) {
		b := &BeaconBlock{version: version.Bellatrix, body: &BeaconBlockBody{version: version.Bellatrix}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Bellatrix, sb.Version())
	})
	t.Run("BellatrixBlind", func(t *testing.T) {
		b := &BeaconBlock{version: version.Bellatrix, body: &BeaconBlockBody{version: version.Bellatrix, isBlinded: true}}
		sb, err := BuildSignedBeaconBlock(b, sig[:])
		require.NoError(t, err)
		assert.DeepEqual(t, sig, sb.Signature())
		assert.Equal(t, version.Bellatrix, sb.Version())
		assert.Equal(t, true, sb.IsBlinded())
	})
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
		altairBlock := &zond.SignedBeaconBlockAltair{
			Block: &zond.BeaconBlockAltair{
				Body: &zond.BeaconBlockBodyAltair{}}}
		blk, err := NewSignedBeaconBlock(altairBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, nil)
		require.Equal(t, true, errors.Is(err, errNonBlindedSignedBeaconBlock))
	})
	t.Run("payload header root and payload root mismatch", func(t *testing.T) {
		blockHash := bytesutil.Bytes32(1)
		payload := &enginev1.ExecutionPayload{
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
		wrapped, err := WrappedExecutionPayload(payload)
		require.NoError(t, err)
		header, err := PayloadToHeader(wrapped)
		require.NoError(t, err)
		blindedBlock := &zond.SignedBlindedBeaconBlockBellatrix{
			Block: &zond.BlindedBeaconBlockBellatrix{
				Body: &zond.BlindedBeaconBlockBodyBellatrix{}}}

		// Modify the header.
		header.GasUsed += 1
		blindedBlock.Block.Body.ExecutionPayloadHeader = header

		blk, err := NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		_, err = BuildSignedBeaconBlockFromExecutionPayload(blk, payload)
		require.ErrorContains(t, "roots do not match", err)
	})
	t.Run("ok", func(t *testing.T) {
		payload := &enginev1.ExecutionPayload{
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
		wrapped, err := WrappedExecutionPayload(payload)
		require.NoError(t, err)
		header, err := PayloadToHeader(wrapped)
		require.NoError(t, err)
		blindedBlock := &zond.SignedBlindedBeaconBlockBellatrix{
			Block: &zond.BlindedBeaconBlockBellatrix{
				Body: &zond.BlindedBeaconBlockBodyBellatrix{}}}
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
