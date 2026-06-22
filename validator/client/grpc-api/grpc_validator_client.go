package grpc_api

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcValidatorClient struct {
	beaconNodeValidatorClient qrysmpb.BeaconNodeValidatorClient
}

func (c *grpcValidatorClient) GetDuties(ctx context.Context, in *qrysmpb.DutiesRequest) (*qrysmpb.DutiesResponse, error) {
	return c.beaconNodeValidatorClient.GetDuties(ctx, in)
}

func (c *grpcValidatorClient) CheckDoppelGanger(ctx context.Context, in *qrysmpb.DoppelGangerRequest) (*qrysmpb.DoppelGangerResponse, error) {
	return c.beaconNodeValidatorClient.CheckDoppelGanger(ctx, in)
}

func (c *grpcValidatorClient) DomainData(ctx context.Context, in *qrysmpb.DomainRequest) (*qrysmpb.DomainResponse, error) {
	return c.beaconNodeValidatorClient.DomainData(ctx, in)
}

func (c *grpcValidatorClient) GetAttestationData(ctx context.Context, in *qrysmpb.AttestationDataRequest) (*qrysmpb.AttestationData, error) {
	return c.beaconNodeValidatorClient.GetAttestationData(ctx, in)
}

func (c *grpcValidatorClient) GetBeaconBlock(ctx context.Context, in *qrysmpb.BlockRequest) (*qrysmpb.GenericBeaconBlock, error) {
	return c.beaconNodeValidatorClient.GetBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) GetFeeRecipientByPubKey(ctx context.Context, in *qrysmpb.FeeRecipientByPubKeyRequest) (*qrysmpb.FeeRecipientByPubKeyResponse, error) {
	return c.beaconNodeValidatorClient.GetFeeRecipientByPubKey(ctx, in)
}

func (c *grpcValidatorClient) GetSyncCommitteeContribution(ctx context.Context, in *qrysmpb.SyncCommitteeContributionRequest) (*qrysmpb.SyncCommitteeContribution, error) {
	return c.beaconNodeValidatorClient.GetSyncCommitteeContribution(ctx, in)
}

func (c *grpcValidatorClient) GetSyncMessageBlockRoot(ctx context.Context, in *emptypb.Empty) (*qrysmpb.SyncMessageBlockRootResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncMessageBlockRoot(ctx, in)
}

func (c *grpcValidatorClient) GetSyncSubcommitteeIndex(ctx context.Context, in *qrysmpb.SyncSubcommitteeIndexRequest) (*qrysmpb.SyncSubcommitteeIndexResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncSubcommitteeIndex(ctx, in)
}

func (c *grpcValidatorClient) MultipleValidatorStatus(ctx context.Context, in *qrysmpb.MultipleValidatorStatusRequest) (*qrysmpb.MultipleValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.MultipleValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) PrepareBeaconProposer(ctx context.Context, in *qrysmpb.PrepareBeaconProposerRequest) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.PrepareBeaconProposer(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestation(ctx context.Context, in *qrysmpb.Attestation) (*qrysmpb.AttestResponse, error) {
	return c.beaconNodeValidatorClient.ProposeAttestation(ctx, in)
}

func (c *grpcValidatorClient) ProposeBeaconBlock(ctx context.Context, in *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error) {
	return c.beaconNodeValidatorClient.ProposeBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) ProposeExit(ctx context.Context, in *qrysmpb.SignedVoluntaryExit) (*qrysmpb.ProposeExitResponse, error) {
	return c.beaconNodeValidatorClient.ProposeExit(ctx, in)
}

func (c *grpcValidatorClient) StreamBlocksAltair(ctx context.Context, in *qrysmpb.StreamBlocksRequest) (qrysmpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.beaconNodeValidatorClient.StreamBlocksAltair(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *qrysmpb.AggregateSelectionRequest) (*qrysmpb.AggregateSelectionResponse, error) {
	return c.beaconNodeValidatorClient.SubmitAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *qrysmpb.SignedAggregateSubmitRequest) (*qrysmpb.SignedAggregateSubmitResponse, error) {
	return c.beaconNodeValidatorClient.SubmitSignedAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *qrysmpb.SignedContributionAndProof) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSignedContributionAndProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSyncMessage(ctx context.Context, in *qrysmpb.SyncCommitteeMessage) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSyncMessage(ctx, in)
}

func (c *grpcValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *qrysmpb.SignedValidatorRegistrationsV1) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitValidatorRegistrations(ctx, in)
}

func (c *grpcValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *qrysmpb.CommitteeSubnetsSubscribeRequest, _ []primitives.ValidatorIndex) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.SubscribeCommitteeSubnets(ctx, in)
}

func (c *grpcValidatorClient) ValidatorIndex(ctx context.Context, in *qrysmpb.ValidatorIndexRequest) (*qrysmpb.ValidatorIndexResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorIndex(ctx, in)
}

func (c *grpcValidatorClient) ValidatorStatus(ctx context.Context, in *qrysmpb.ValidatorStatusRequest) (*qrysmpb.ValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) WaitForActivation(ctx context.Context, in *qrysmpb.ValidatorActivationRequest) (qrysmpb.BeaconNodeValidator_WaitForActivationClient, error) {
	return c.beaconNodeValidatorClient.WaitForActivation(ctx, in)
}

// Deprecated: Do not use.
func (c *grpcValidatorClient) WaitForChainStart(ctx context.Context, in *emptypb.Empty) (*qrysmpb.ChainStartResponse, error) {
	stream, err := c.beaconNodeValidatorClient.WaitForChainStart(ctx, in)
	if err != nil {
		return nil, errors.Wrap(
			iface.ErrConnectionIssue,
			errors.Wrap(err, "could not setup beacon chain ChainStart streaming client").Error(),
		)
	}

	return stream.Recv()
}

func (c *grpcValidatorClient) AssignValidatorToSubnet(ctx context.Context, in *qrysmpb.AssignValidatorToSubnetRequest) (*emptypb.Empty, error) {
	return c.beaconNodeValidatorClient.AssignValidatorToSubnet(ctx, in)
}
func (c *grpcValidatorClient) SignaturesAndAggregationBits(
	ctx context.Context,
	in *qrysmpb.SignaturesAndAggregationBitsRequest,
) (*qrysmpb.SignaturesAndAggregationBitsResponse, error) {
	return c.beaconNodeValidatorClient.SignaturesAndAggregationBits(ctx, in)
}

func NewGrpcValidatorClient(cc grpc.ClientConnInterface) iface.ValidatorClient {
	return &grpcValidatorClient{qrysmpb.NewBeaconNodeValidatorClient(cc)}
}
