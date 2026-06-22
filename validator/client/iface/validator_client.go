package iface

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ValidatorClient interface {
	GetDuties(ctx context.Context, in *qrysmpb.DutiesRequest) (*qrysmpb.DutiesResponse, error)
	DomainData(ctx context.Context, in *qrysmpb.DomainRequest) (*qrysmpb.DomainResponse, error)
	WaitForChainStart(ctx context.Context, in *emptypb.Empty) (*qrysmpb.ChainStartResponse, error)
	WaitForActivation(ctx context.Context, in *qrysmpb.ValidatorActivationRequest) (qrysmpb.BeaconNodeValidator_WaitForActivationClient, error)
	ValidatorIndex(ctx context.Context, in *qrysmpb.ValidatorIndexRequest) (*qrysmpb.ValidatorIndexResponse, error)
	ValidatorStatus(ctx context.Context, in *qrysmpb.ValidatorStatusRequest) (*qrysmpb.ValidatorStatusResponse, error)
	MultipleValidatorStatus(ctx context.Context, in *qrysmpb.MultipleValidatorStatusRequest) (*qrysmpb.MultipleValidatorStatusResponse, error)
	GetBeaconBlock(ctx context.Context, in *qrysmpb.BlockRequest) (*qrysmpb.GenericBeaconBlock, error)
	ProposeBeaconBlock(ctx context.Context, in *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error)
	PrepareBeaconProposer(ctx context.Context, in *qrysmpb.PrepareBeaconProposerRequest) (*emptypb.Empty, error)
	GetFeeRecipientByPubKey(ctx context.Context, in *qrysmpb.FeeRecipientByPubKeyRequest) (*qrysmpb.FeeRecipientByPubKeyResponse, error)
	GetAttestationData(ctx context.Context, in *qrysmpb.AttestationDataRequest) (*qrysmpb.AttestationData, error)
	ProposeAttestation(ctx context.Context, in *qrysmpb.Attestation) (*qrysmpb.AttestResponse, error)
	SubmitAggregateSelectionProof(ctx context.Context, in *qrysmpb.AggregateSelectionRequest) (*qrysmpb.AggregateSelectionResponse, error)
	SubmitSignedAggregateSelectionProof(ctx context.Context, in *qrysmpb.SignedAggregateSubmitRequest) (*qrysmpb.SignedAggregateSubmitResponse, error)
	ProposeExit(ctx context.Context, in *qrysmpb.SignedVoluntaryExit) (*qrysmpb.ProposeExitResponse, error)
	SubscribeCommitteeSubnets(ctx context.Context, in *qrysmpb.CommitteeSubnetsSubscribeRequest, validatorIndices []primitives.ValidatorIndex) (*emptypb.Empty, error)
	CheckDoppelGanger(ctx context.Context, in *qrysmpb.DoppelGangerRequest) (*qrysmpb.DoppelGangerResponse, error)
	GetSyncMessageBlockRoot(ctx context.Context, in *emptypb.Empty) (*qrysmpb.SyncMessageBlockRootResponse, error)
	SubmitSyncMessage(ctx context.Context, in *qrysmpb.SyncCommitteeMessage) (*emptypb.Empty, error)
	GetSyncSubcommitteeIndex(ctx context.Context, in *qrysmpb.SyncSubcommitteeIndexRequest) (*qrysmpb.SyncSubcommitteeIndexResponse, error)
	GetSyncCommitteeContribution(ctx context.Context, in *qrysmpb.SyncCommitteeContributionRequest) (*qrysmpb.SyncCommitteeContribution, error)
	SubmitSignedContributionAndProof(ctx context.Context, in *qrysmpb.SignedContributionAndProof) (*emptypb.Empty, error)
	StreamBlocksAltair(ctx context.Context, in *qrysmpb.StreamBlocksRequest) (qrysmpb.BeaconNodeValidator_StreamBlocksAltairClient, error)
	SubmitValidatorRegistrations(ctx context.Context, in *qrysmpb.SignedValidatorRegistrationsV1) (*emptypb.Empty, error)
}
