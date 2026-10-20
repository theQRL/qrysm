package validator

import (
	"context"
	"fmt"
	"testing"

	chainmock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	buildermock "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	p2pmock "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type proposalValidationReceiver struct {
	*chainmock.ChainService
	received int
}

func (r *proposalValidationReceiver) ReceiveBlock(context.Context, interfaces.ReadOnlySignedBeaconBlock, [32]byte) error {
	r.received++
	return nil
}

type proposalValidationBuilder struct {
	*buildermock.MockBuilderService
	submitted int
}

func (b *proposalValidationBuilder) SubmitBlindedBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, error) {
	b.submitted++
	return b.MockBuilderService.SubmitBlindedBlock(ctx, block)
}

func proposalValidationFixture(t *testing.T, blinded bool) (*qrysmpb.GenericSignedBeaconBlock, map[string]*[]byte, *proposalValidationBuilder) {
	t.Helper()
	full := util.NewBeaconBlockZond()
	builder := &proposalValidationBuilder{MockBuilderService: &buildermock.MockBuilderService{
		HasConfigured: true,
		PayloadZond:   full.Block.Body.ExecutionPayload,
	}}
	if blinded {
		block := util.NewBlindedBeaconBlockZond()
		payload, err := blocks.WrappedExecutionPayloadZond(builder.PayloadZond, 0)
		require.NoError(t, err)
		block.Block.Body.ExecutionPayloadHeader, err = blocks.PayloadToHeaderZond(payload)
		require.NoError(t, err)
		return &qrysmpb.GenericSignedBeaconBlock{Block: &qrysmpb.GenericSignedBeaconBlock_BlindedZond{BlindedZond: block}}, map[string]*[]byte{
			"signature": &block.Signature, "parent root": &block.Block.ParentRoot,
			"state root": &block.Block.StateRoot, "randao reveal": &block.Block.Body.RandaoReveal,
			"graffiti": &block.Block.Body.Graffiti,
		}, builder
	}
	return &qrysmpb.GenericSignedBeaconBlock{Block: &qrysmpb.GenericSignedBeaconBlock_Zond{Zond: full}}, map[string]*[]byte{
		"signature": &full.Signature, "parent root": &full.Block.ParentRoot,
		"state root": &full.Block.StateRoot, "randao reveal": &full.Block.Body.RandaoReveal,
		"graffiti": &full.Block.Body.Graffiti,
	}, builder
}

func TestProposeBeaconBlock_InvalidFieldLengths(t *testing.T) {
	for _, blinded := range []bool{false, true} {
		for _, field := range []string{"signature", "parent root", "state root", "randao reveal", "graffiti"} {
			for _, size := range []string{"empty", "short", "long"} {
				t.Run(fmt.Sprintf("blinded=%t/%s/%s", blinded, field, size), func(t *testing.T) {
					req, fields, builder := proposalValidationFixture(t, blinded)
					value := fields[field]
					switch size {
					case "empty":
						*value = nil
					case "short":
						*value = (*value)[:len(*value)-1]
					case "long":
						*value = append(*value, 0x42)
					}
					// Preserve the malformed lengths through the same protobuf decoding
					// used by gRPC before exercising the proposal handler.
					wire, err := proto.Marshal(req)
					require.NoError(t, err)
					decoded := &qrysmpb.GenericSignedBeaconBlock{}
					require.NoError(t, proto.Unmarshal(wire, decoded))
					before := proto.Clone(decoded)
					broadcaster := &p2pmock.MockBroadcaster{}
					receiver := &proposalValidationReceiver{}
					server := &Server{P2P: broadcaster, BlockReceiver: receiver, BlockBuilder: builder, BlockNotifier: &chainmock.MockBlockNotifier{}}
					response, err := server.ProposeBeaconBlock(context.Background(), decoded)
					assert.Equal(t, codes.InvalidArgument, status.Code(err))
					assert.ErrorContains(t, field, err)
					assert.Equal(t, true, response == nil)
					assert.Equal(t, false, broadcaster.BroadcastCalled, "reject before broadcasting")
					assert.Equal(t, 0, receiver.received, "reject before block processing")
					assert.Equal(t, 0, builder.submitted, "reject before contacting the builder")
					assert.Equal(t, true, proto.Equal(before, decoded), "rejection must preserve the request")
				})
			}
		}
	}
}

func TestProposeBeaconBlock_ValidFieldLengths(t *testing.T) {
	for _, blinded := range []bool{false, true} {
		t.Run(fmt.Sprintf("blinded=%t", blinded), func(t *testing.T) {
			req, _, builder := proposalValidationFixture(t, blinded)
			original, err := blocks.NewSignedBeaconBlock(req.Block)
			require.NoError(t, err)
			wantRoot, err := original.Block().HashTreeRoot()
			require.NoError(t, err)
			broadcaster := &p2pmock.MockBroadcaster{}
			receiver := &proposalValidationReceiver{}
			server := &Server{P2P: broadcaster, BlockReceiver: receiver, BlockBuilder: builder, BlockNotifier: &chainmock.MockBlockNotifier{}}
			response, err := server.ProposeBeaconBlock(context.Background(), req)
			require.NoError(t, err)
			assert.DeepEqual(t, wantRoot[:], response.BlockRoot)
			assert.Equal(t, 1, len(broadcaster.BroadcastMessages))
			assert.Equal(t, 1, receiver.received)
			wantSubmissions := 0
			if blinded {
				wantSubmissions = 1
			}
			assert.Equal(t, wantSubmissions, builder.submitted)
		})
	}
}

func TestProposeBeaconBlock_MissingBlock(t *testing.T) {
	for name, req := range map[string]*qrysmpb.GenericSignedBeaconBlock{
		"nil request":       nil,
		"empty request":     {},
		"nil full oneof":    {Block: (*qrysmpb.GenericSignedBeaconBlock_Zond)(nil)},
		"nil blinded oneof": {Block: (*qrysmpb.GenericSignedBeaconBlock_BlindedZond)(nil)},
		"nil full block":    {Block: &qrysmpb.GenericSignedBeaconBlock_Zond{}},
		"nil blinded block": {Block: &qrysmpb.GenericSignedBeaconBlock_BlindedZond{}},
	} {
		t.Run(name, func(t *testing.T) {
			server := &Server{}
			response, err := server.ProposeBeaconBlock(context.Background(), req)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Equal(t, true, response == nil)
		})
	}
}
