package grpc_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	"google.golang.org/grpc"
)

type grpcValidatorClient struct {
	beaconNodeValidatorClient zondpb.BeaconNodeValidatorClient
}

func (c *grpcValidatorClient) GetDuties(ctx context.Context, in *zondpb.DutiesRequest) (*zondpb.DutiesResponse, error) {
	return c.beaconNodeValidatorClient.GetDuties(ctx, in)
}

func (c *grpcValidatorClient) CheckDoppelGanger(ctx context.Context, in *zondpb.DoppelGangerRequest) (*zondpb.DoppelGangerResponse, error) {
	return c.beaconNodeValidatorClient.CheckDoppelGanger(ctx, in)
}

func (c *grpcValidatorClient) DomainData(ctx context.Context, in *zondpb.DomainRequest) (*zondpb.DomainResponse, error) {
	return c.beaconNodeValidatorClient.DomainData(ctx, in)
}

func (c *grpcValidatorClient) GetAttestationData(ctx context.Context, in *zondpb.AttestationDataRequest) (*zondpb.AttestationData, error) {
	return c.beaconNodeValidatorClient.GetAttestationData(ctx, in)
}

func (c *grpcValidatorClient) GetBeaconBlock(ctx context.Context, in *zondpb.BlockRequest) (*zondpb.GenericBeaconBlock, error) {
	return c.beaconNodeValidatorClient.GetBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) GetFeeRecipientByPubKey(ctx context.Context, in *zondpb.FeeRecipientByPubKeyRequest) (*zondpb.FeeRecipientByPubKeyResponse, error) {
	return c.beaconNodeValidatorClient.GetFeeRecipientByPubKey(ctx, in)
}

func (c *grpcValidatorClient) GetSyncCommitteeContribution(ctx context.Context, in *zondpb.SyncCommitteeContributionRequest) (*zondpb.SyncCommitteeContribution, error) {
	return c.beaconNodeValidatorClient.GetSyncCommitteeContribution(ctx, in)
}

func (c *grpcValidatorClient) GetSyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*zondpb.SyncMessageBlockRootResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncMessageBlockRoot(ctx, in)
}

func (c *grpcValidatorClient) GetSyncSubcommitteeIndex(ctx context.Context, in *zondpb.SyncSubcommitteeIndexRequest) (*zondpb.SyncSubcommitteeIndexResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncSubcommitteeIndex(ctx, in)
}

func (c *grpcValidatorClient) MultipleValidatorStatus(ctx context.Context, in *zondpb.MultipleValidatorStatusRequest) (*zondpb.MultipleValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.MultipleValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) PrepareBeaconProposer(ctx context.Context, in *zondpb.PrepareBeaconProposerRequest) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.PrepareBeaconProposer(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestation(ctx context.Context, in *zondpb.Attestation) (*zondpb.AttestResponse, error) {
	return c.beaconNodeValidatorClient.ProposeAttestation(ctx, in)
}

func (c *grpcValidatorClient) ProposeBeaconBlock(ctx context.Context, in *zondpb.GenericSignedBeaconBlock) (*zondpb.ProposeResponse, error) {
	return c.beaconNodeValidatorClient.ProposeBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) ProposeExit(ctx context.Context, in *zondpb.SignedVoluntaryExit) (*zondpb.ProposeExitResponse, error) {
	return c.beaconNodeValidatorClient.ProposeExit(ctx, in)
}

func (c *grpcValidatorClient) StreamBlocksAltair(ctx context.Context, in *zondpb.StreamBlocksRequest) (zondpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.beaconNodeValidatorClient.StreamBlocksAltair(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *zondpb.AggregateSelectionRequest) (*zondpb.AggregateSelectionResponse, error) {
	return c.beaconNodeValidatorClient.SubmitAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *zondpb.SignedAggregateSubmitRequest) (*zondpb.SignedAggregateSubmitResponse, error) {
	return c.beaconNodeValidatorClient.SubmitSignedAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *zondpb.SignedContributionAndProof) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSignedContributionAndProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSyncMessage(ctx context.Context, in *zondpb.SyncCommitteeMessage) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSyncMessage(ctx, in)
}

func (c *grpcValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *zondpb.SignedValidatorRegistrationsV1) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitValidatorRegistrations(ctx, in)
}

func (c *grpcValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *zondpb.CommitteeSubnetsSubscribeRequest, _ []primitives.ValidatorIndex) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubscribeCommitteeSubnets(ctx, in)
}

func (c *grpcValidatorClient) ValidatorIndex(ctx context.Context, in *zondpb.ValidatorIndexRequest) (*zondpb.ValidatorIndexResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorIndex(ctx, in)
}

func (c *grpcValidatorClient) ValidatorStatus(ctx context.Context, in *zondpb.ValidatorStatusRequest) (*zondpb.ValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) WaitForActivation(ctx context.Context, in *zondpb.ValidatorActivationRequest) (zondpb.BeaconNodeValidator_WaitForActivationClient, error) {
	return c.beaconNodeValidatorClient.WaitForActivation(ctx, in)
}

// Deprecated: Do not use.
func (c *grpcValidatorClient) WaitForChainStart(ctx context.Context, in *empty.Empty) (*zondpb.ChainStartResponse, error) {
	stream, err := c.beaconNodeValidatorClient.WaitForChainStart(ctx, in)
	if err != nil {
		return nil, errors.Wrap(
			iface.ErrConnectionIssue,
			errors.Wrap(err, "could not setup beacon chain ChainStart streaming client").Error(),
		)
	}

	return stream.Recv()
}

func (c *grpcValidatorClient) AssignValidatorToSubnet(ctx context.Context, in *zondpb.AssignValidatorToSubnetRequest) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.AssignValidatorToSubnet(ctx, in)
}
func (c *grpcValidatorClient) AggregatedSigAndAggregationBits(
	ctx context.Context,
	in *zondpb.AggregatedSigAndAggregationBitsRequest,
) (*zondpb.AggregatedSigAndAggregationBitsResponse, error) {
	return c.beaconNodeValidatorClient.AggregatedSigAndAggregationBits(ctx, in)
}

func NewGrpcValidatorClient(cc grpc.ClientConnInterface) iface.ValidatorClient {
	return &grpcValidatorClient{zondpb.NewBeaconNodeValidatorClient(cc)}
}
