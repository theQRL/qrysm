package beacon_api

import (
	"context"
	"net/http"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
)

type beaconApiValidatorClient struct {
	genesisProvider         genesisProvider
	dutiesProvider          dutiesProvider
	stateValidatorsProvider stateValidatorsProvider
	jsonRestHandler         jsonRestHandler
	beaconBlockConverter    beaconBlockConverter
}

func NewBeaconApiValidatorClient(host string, timeout time.Duration) iface.ValidatorClient {
	jsonRestHandler := beaconApiJsonRestHandler{
		httpClient: http.Client{Timeout: timeout},
		host:       host,
	}

	return &beaconApiValidatorClient{
		genesisProvider:         beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler},
		dutiesProvider:          beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler},
		stateValidatorsProvider: beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler},
		jsonRestHandler:         jsonRestHandler,
		beaconBlockConverter:    beaconApiBeaconBlockConverter{},
	}
}

func (c *beaconApiValidatorClient) GetDuties(ctx context.Context, in *zondpb.DutiesRequest) (*zondpb.DutiesResponse, error) {
	return c.getDuties(ctx, in)
}

func (c *beaconApiValidatorClient) CheckDoppelGanger(ctx context.Context, in *zondpb.DoppelGangerRequest) (*zondpb.DoppelGangerResponse, error) {
	return c.checkDoppelGanger(ctx, in)
}

func (c *beaconApiValidatorClient) DomainData(ctx context.Context, in *zondpb.DomainRequest) (*zondpb.DomainResponse, error) {
	if len(in.Domain) != 4 {
		return nil, errors.Errorf("invalid domain type: %s", hexutil.Encode(in.Domain))
	}

	domainType := bytesutil.ToBytes4(in.Domain)
	return c.getDomainData(ctx, in.Epoch, domainType)
}

func (c *beaconApiValidatorClient) GetAttestationData(ctx context.Context, in *zondpb.AttestationDataRequest) (*zondpb.AttestationData, error) {
	if in == nil {
		return nil, errors.New("GetAttestationData received nil argument `in`")
	}

	return c.getAttestationData(ctx, in.Slot, in.CommitteeIndex)
}

func (c *beaconApiValidatorClient) GetBeaconBlock(ctx context.Context, in *zondpb.BlockRequest) (*zondpb.GenericBeaconBlock, error) {
	return c.getBeaconBlock(ctx, in.Slot, in.RandaoReveal, in.Graffiti)
}

func (c *beaconApiValidatorClient) GetFeeRecipientByPubKey(_ context.Context, _ *zondpb.FeeRecipientByPubKeyRequest) (*zondpb.FeeRecipientByPubKeyResponse, error) {
	return nil, nil
}

func (c *beaconApiValidatorClient) GetSyncCommitteeContribution(ctx context.Context, in *zondpb.SyncCommitteeContributionRequest) (*zondpb.SyncCommitteeContribution, error) {
	return c.getSyncCommitteeContribution(ctx, in)
}

func (c *beaconApiValidatorClient) GetSyncMessageBlockRoot(ctx context.Context, _ *empty.Empty) (*zondpb.SyncMessageBlockRootResponse, error) {
	return c.getSyncMessageBlockRoot(ctx)
}

func (c *beaconApiValidatorClient) GetSyncSubcommitteeIndex(ctx context.Context, in *zondpb.SyncSubcommitteeIndexRequest) (*zondpb.SyncSubcommitteeIndexResponse, error) {
	return c.getSyncSubcommitteeIndex(ctx, in)
}

func (c *beaconApiValidatorClient) MultipleValidatorStatus(ctx context.Context, in *zondpb.MultipleValidatorStatusRequest) (*zondpb.MultipleValidatorStatusResponse, error) {
	return c.multipleValidatorStatus(ctx, in)
}

func (c *beaconApiValidatorClient) PrepareBeaconProposer(ctx context.Context, in *zondpb.PrepareBeaconProposerRequest) (*empty.Empty, error) {
	return new(empty.Empty), c.prepareBeaconProposer(ctx, in.Recipients)
}

func (c *beaconApiValidatorClient) ProposeAttestation(ctx context.Context, in *zondpb.Attestation) (*zondpb.AttestResponse, error) {
	return c.proposeAttestation(ctx, in)
}

func (c *beaconApiValidatorClient) ProposeBeaconBlock(ctx context.Context, in *zondpb.GenericSignedBeaconBlock) (*zondpb.ProposeResponse, error) {
	return c.proposeBeaconBlock(ctx, in)
}

func (c *beaconApiValidatorClient) ProposeExit(ctx context.Context, in *zondpb.SignedVoluntaryExit) (*zondpb.ProposeExitResponse, error) {
	return c.proposeExit(ctx, in)
}

func (c *beaconApiValidatorClient) StreamBlocksAltair(ctx context.Context, in *zondpb.StreamBlocksRequest) (zondpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.streamBlocks(ctx, in, time.Second), nil
}

func (c *beaconApiValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *zondpb.AggregateSelectionRequest) (*zondpb.AggregateSelectionResponse, error) {
	return c.submitAggregateSelectionProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *zondpb.SignedAggregateSubmitRequest) (*zondpb.SignedAggregateSubmitResponse, error) {
	return c.submitSignedAggregateSelectionProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *zondpb.SignedContributionAndProof) (*empty.Empty, error) {
	return new(empty.Empty), c.submitSignedContributionAndProof(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitSyncMessage(ctx context.Context, in *zondpb.SyncCommitteeMessage) (*empty.Empty, error) {
	return new(empty.Empty), c.submitSyncMessage(ctx, in)
}

func (c *beaconApiValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *zondpb.SignedValidatorRegistrationsV1) (*empty.Empty, error) {
	return new(empty.Empty), c.submitValidatorRegistrations(ctx, in.Messages)
}

func (c *beaconApiValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *zondpb.CommitteeSubnetsSubscribeRequest, validatorIndices []primitives.ValidatorIndex) (*empty.Empty, error) {
	return new(empty.Empty), c.subscribeCommitteeSubnets(ctx, in, validatorIndices)
}

func (c *beaconApiValidatorClient) ValidatorIndex(ctx context.Context, in *zondpb.ValidatorIndexRequest) (*zondpb.ValidatorIndexResponse, error) {
	return c.validatorIndex(ctx, in)
}

func (c *beaconApiValidatorClient) ValidatorStatus(ctx context.Context, in *zondpb.ValidatorStatusRequest) (*zondpb.ValidatorStatusResponse, error) {
	return c.validatorStatus(ctx, in)
}

func (c *beaconApiValidatorClient) WaitForActivation(ctx context.Context, in *zondpb.ValidatorActivationRequest) (zondpb.BeaconNodeValidator_WaitForActivationClient, error) {
	return c.waitForActivation(ctx, in)
}

// Deprecated: Do not use.
func (c *beaconApiValidatorClient) WaitForChainStart(ctx context.Context, _ *empty.Empty) (*zondpb.ChainStartResponse, error) {
	return c.waitForChainStart(ctx)
}
