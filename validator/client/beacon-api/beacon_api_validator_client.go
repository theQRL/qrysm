package beacon_api

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconApiValidatorClient struct {
	genesisProvider         genesisProvider
	dutiesProvider          dutiesProvider
	stateValidatorsProvider stateValidatorsProvider
	jsonRestHandler         jsonRestHandler
	beaconBlockConverter    beaconBlockConverter
}

func NewBeaconApiValidatorClient(host string, timeout time.Duration) iface.ValidatorClient {
	jsonRestHandler := newBeaconAPIJSONRestHandler(host, timeout)

	return &beaconApiValidatorClient{
		genesisProvider:         beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler},
		dutiesProvider:          beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler},
		stateValidatorsProvider: beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler},
		jsonRestHandler:         jsonRestHandler,
		beaconBlockConverter:    beaconApiBeaconBlockConverter{},
	}
}

func (c *beaconApiValidatorClient) GetDuties(ctx context.Context, in *qrysmpb.DutiesRequest) (*qrysmpb.DutiesResponse, error) {
	return c.getDuties(ctx, in)
}

func (c *beaconApiValidatorClient) CheckDoppelGanger(ctx context.Context, in *qrysmpb.DoppelGangerRequest) (*qrysmpb.DoppelGangerResponse, error) {
	return c.checkDoppelGanger(ctx, in)
}

func (c *beaconApiValidatorClient) DomainData(ctx context.Context, in *qrysmpb.DomainRequest) (*qrysmpb.DomainResponse, error) {
	if len(in.Domain) != 4 {
		return nil, errors.Errorf("invalid domain type: %s", hexutil.Encode(in.Domain))
	}

	domainType := bytesutil.ToBytes4(in.Domain)
	return c.getDomainData(ctx, in.Epoch, domainType)
}

func (c *beaconApiValidatorClient) GetAttestationData(ctx context.Context, in *qrysmpb.AttestationDataRequest) (*qrysmpb.AttestationData, error) {
	if in == nil {
		return nil, errors.New("GetAttestationData received nil argument `in`")
	}

	return c.getAttestationData(ctx, in.Slot, in.CommitteeIndex)
}

func (c *beaconApiValidatorClient) GetBeaconBlock(ctx context.Context, in *qrysmpb.BlockRequest) (*qrysmpb.GenericBeaconBlock, error) {
	return c.getBeaconBlock(ctx, in.Slot, in.RandaoReveal, in.Graffiti)
}

func (c *beaconApiValidatorClient) GetFeeRecipientByPubKey(_ context.Context, _ *qrysmpb.FeeRecipientByPubKeyRequest) (*qrysmpb.FeeRecipientByPubKeyResponse, error) {
	return nil, nil
}

func (c *beaconApiValidatorClient) GetSyncCommitteeContribution(ctx context.Context, in *qrysmpb.SyncCommitteeContributionRequest) (*qrysmpb.SyncCommitteeContribution, error) {
	return c.getSyncCommitteeContribution(ctx, in)
}

func (c *beaconApiValidatorClient) GetSyncMessageBlockRoot(ctx context.Context, _ *emptypb.Empty) (*qrysmpb.SyncMessageBlockRootResponse, error) {
	return c.getSyncMessageBlockRoot(ctx)
}

func (c *beaconApiValidatorClient) GetSyncSubcommitteeIndex(ctx context.Context, in *qrysmpb.SyncSubcommitteeIndexRequest) (*qrysmpb.SyncSubcommitteeIndexResponse, error) {
	return c.getSyncSubcommitteeIndex(ctx, in)
}

func (c *beaconApiValidatorClient) MultipleValidatorStatus(ctx context.Context, in *qrysmpb.MultipleValidatorStatusRequest) (*qrysmpb.MultipleValidatorStatusResponse, error) {
	return c.multipleValidatorStatus(ctx, in)
}

func (c *beaconApiValidatorClient) PrepareBeaconProposer(ctx context.Context, in *qrysmpb.PrepareBeaconProposerRequest) (*emptypb.Empty, error) {
	return new(emptypb.Empty), c.prepareBeaconProposer(ctx, in.Recipients)
}

func (c *beaconApiValidatorClient) ProposeAttestation(ctx context.Context, in *qrysmpb.Attestation) (*qrysmpb.AttestResponse, error) {
	return c.proposeAttestation(ctx, in)
}

func (c *beaconApiValidatorClient) ProposeBeaconBlock(ctx context.Context, in *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error) {
	return c.proposeBeaconBlock(ctx, in)
}

func (c *beaconApiValidatorClient) ProposeExit(ctx context.Context, in *qrysmpb.SignedVoluntaryExit) (*qrysmpb.ProposeExitResponse, error) {
	return c.proposeExit(ctx, in)
}

func (c *beaconApiValidatorClient) StreamBlocksAltair(ctx context.Context, in *qrysmpb.StreamBlocksRequest) (qrysmpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.streamBlocks(ctx, in, time.Second), nil
}

func (c *beaconApiValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *qrysmpb.AggregateSelectionRequest) (*qrysmpb.AggregateSelectionResponse, error) {
	return c.submitAggregateSelectionProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *qrysmpb.SignedAggregateSubmitRequest) (*qrysmpb.SignedAggregateSubmitResponse, error) {
	return c.submitSignedAggregateSelectionProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *qrysmpb.SignedContributionAndProof) (*emptypb.Empty, error) {
	return new(emptypb.Empty), c.submitSignedContributionAndProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSyncMessage(ctx context.Context, in *qrysmpb.SyncCommitteeMessage) (*emptypb.Empty, error) {
	return new(emptypb.Empty), c.submitSyncMessage(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *qrysmpb.SignedValidatorRegistrationsV1) (*emptypb.Empty, error) {
	return new(emptypb.Empty), c.submitValidatorRegistrations(ctx, in.Messages)
}

func (c *beaconApiValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *qrysmpb.CommitteeSubnetsSubscribeRequest, validatorIndices []primitives.ValidatorIndex) (*emptypb.Empty, error) {
	return new(emptypb.Empty), c.subscribeCommitteeSubnets(ctx, in, validatorIndices)
}

func (c *beaconApiValidatorClient) ValidatorIndex(ctx context.Context, in *qrysmpb.ValidatorIndexRequest) (*qrysmpb.ValidatorIndexResponse, error) {
	return c.validatorIndex(ctx, in)
}

func (c *beaconApiValidatorClient) ValidatorStatus(ctx context.Context, in *qrysmpb.ValidatorStatusRequest) (*qrysmpb.ValidatorStatusResponse, error) {
	return c.validatorStatus(ctx, in)
}

func (c *beaconApiValidatorClient) WaitForActivation(ctx context.Context, in *qrysmpb.ValidatorActivationRequest) (qrysmpb.BeaconNodeValidator_WaitForActivationClient, error) {
	return c.waitForActivation(ctx, in)
}

// Deprecated: Do not use.
func (c *beaconApiValidatorClient) WaitForChainStart(ctx context.Context, _ *emptypb.Empty) (*qrysmpb.ChainStartResponse, error) {
	return c.waitForChainStart(ctx)
}
