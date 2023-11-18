// Package operation contains types for block operation-specific events fired during the runtime of a beacon node.
package operation

import (
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

const (
	// UnaggregatedAttReceived is sent after an unaggregated attestation object has been received
	// from the outside world. (eg. in RPC or sync)
	UnaggregatedAttReceived = iota + 1

	// AggregatedAttReceived is sent after an aggregated attestation object has been received
	// from the outside world. (eg. in sync)
	AggregatedAttReceived

	// ExitReceived is sent after an voluntary exit object has been received from the outside world (eg in RPC or sync)
	ExitReceived

	// SyncCommitteeContributionReceived is sent after a sync committee contribution object has been received.
	SyncCommitteeContributionReceived

	// DilithiumToExecutionChangeReceived is sent after a Dilithium to execution change object has been received from gossip or rpc.
	DilithiumToExecutionChangeReceived

	// BlobSidecarReceived is sent after a blob sidecar is received from gossip or rpc.
	BlobSidecarReceived = 6
)

// UnAggregatedAttReceivedData is the data sent with UnaggregatedAttReceived events.
type UnAggregatedAttReceivedData struct {
	// Attestation is the unaggregated attestation object.
	Attestation *zondpb.Attestation
}

// AggregatedAttReceivedData is the data sent with AggregatedAttReceived events.
type AggregatedAttReceivedData struct {
	// Attestation is the aggregated attestation object.
	Attestation *zondpb.AggregateAttestationAndProof
}

// ExitReceivedData is the data sent with ExitReceived events.
type ExitReceivedData struct {
	// Exit is the voluntary exit object.
	Exit *zondpb.SignedVoluntaryExit
}

// SyncCommitteeContributionReceivedData is the data sent with SyncCommitteeContributionReceived objects.
type SyncCommitteeContributionReceivedData struct {
	// Contribution is the sync committee contribution object.
	Contribution *zondpb.SignedContributionAndProof
}

// DilithiumToExecutionChangeReceivedData is the data sent with DilithiumToExecutionChangeReceived events.
type DilithiumToExecutionChangeReceivedData struct {
	Change *zondpb.SignedDilithiumToExecutionChange
}

// BlobSidecarReceivedData is the data sent with BlobSidecarReceived events.
type BlobSidecarReceivedData struct {
	Blob *zondpb.SignedBlobSidecar
}
