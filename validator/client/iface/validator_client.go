package iface

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

type ValidatorClient interface {
	GetDuties(ctx context.Context, in *zondpb.DutiesRequest) (*zondpb.DutiesResponse, error)
	DomainData(ctx context.Context, in *zondpb.DomainRequest) (*zondpb.DomainResponse, error)
	WaitForChainStart(ctx context.Context, in *empty.Empty) (*zondpb.ChainStartResponse, error)
	WaitForActivation(ctx context.Context, in *zondpb.ValidatorActivationRequest) (zondpb.BeaconNodeValidator_WaitForActivationClient, error)
	ValidatorIndex(ctx context.Context, in *zondpb.ValidatorIndexRequest) (*zondpb.ValidatorIndexResponse, error)
	ValidatorStatus(ctx context.Context, in *zondpb.ValidatorStatusRequest) (*zondpb.ValidatorStatusResponse, error)
	MultipleValidatorStatus(ctx context.Context, in *zondpb.MultipleValidatorStatusRequest) (*zondpb.MultipleValidatorStatusResponse, error)
	GetBeaconBlock(ctx context.Context, in *zondpb.BlockRequest) (*zondpb.GenericBeaconBlock, error)
	ProposeBeaconBlock(ctx context.Context, in *zondpb.GenericSignedBeaconBlock) (*zondpb.ProposeResponse, error)
	PrepareBeaconProposer(ctx context.Context, in *zondpb.PrepareBeaconProposerRequest) (*empty.Empty, error)
	GetFeeRecipientByPubKey(ctx context.Context, in *zondpb.FeeRecipientByPubKeyRequest) (*zondpb.FeeRecipientByPubKeyResponse, error)
	GetAttestationData(ctx context.Context, in *zondpb.AttestationDataRequest) (*zondpb.AttestationData, error)
	ProposeAttestation(ctx context.Context, in *zondpb.Attestation) (*zondpb.AttestResponse, error)
	SubmitAggregateSelectionProof(ctx context.Context, in *zondpb.AggregateSelectionRequest) (*zondpb.AggregateSelectionResponse, error)
	SubmitSignedAggregateSelectionProof(ctx context.Context, in *zondpb.SignedAggregateSubmitRequest) (*zondpb.SignedAggregateSubmitResponse, error)
	ProposeExit(ctx context.Context, in *zondpb.SignedVoluntaryExit) (*zondpb.ProposeExitResponse, error)
	SubscribeCommitteeSubnets(ctx context.Context, in *zondpb.CommitteeSubnetsSubscribeRequest, validatorIndices []primitives.ValidatorIndex) (*empty.Empty, error)
	CheckDoppelGanger(ctx context.Context, in *zondpb.DoppelGangerRequest) (*zondpb.DoppelGangerResponse, error)
	GetSyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*zondpb.SyncMessageBlockRootResponse, error)
	SubmitSyncMessage(ctx context.Context, in *zondpb.SyncCommitteeMessage) (*empty.Empty, error)
	GetSyncSubcommitteeIndex(ctx context.Context, in *zondpb.SyncSubcommitteeIndexRequest) (*zondpb.SyncSubcommitteeIndexResponse, error)
	GetSyncCommitteeContribution(ctx context.Context, in *zondpb.SyncCommitteeContributionRequest) (*zondpb.SyncCommitteeContribution, error)
	SubmitSignedContributionAndProof(ctx context.Context, in *zondpb.SignedContributionAndProof) (*empty.Empty, error)
	StreamBlocksAltair(ctx context.Context, in *zondpb.StreamBlocksRequest) (zondpb.BeaconNodeValidator_StreamBlocksAltairClient, error)
	SubmitValidatorRegistrations(ctx context.Context, in *zondpb.SignedValidatorRegistrationsV1) (*empty.Empty, error)
}
