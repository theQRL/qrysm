package util

import (
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// NewBeaconBlockZond creates a beacon block with minimum marshalable fields.
func NewBeaconBlockZond() *qrysmpb.SignedBeaconBlockZond {
	return HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{})
}

// NewBlindedBeaconBlockZond creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockZond() *qrysmpb.SignedBlindedBeaconBlockZond {
	return HydrateSignedBlindedBeaconBlockZond(&qrysmpb.SignedBlindedBeaconBlockZond{})
}

// NewBlindedBeaconBlockZondV1 creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockZondV1() *qrlpb.SignedBlindedBeaconBlockZond {
	return HydrateV1SignedBlindedBeaconBlockZond(&qrlpb.SignedBlindedBeaconBlockZond{})
}
