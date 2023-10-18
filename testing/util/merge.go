package util

import (
	v2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// NewBeaconBlockBellatrix creates a beacon block with minimum marshalable fields.
func NewBeaconBlockBellatrix() *zondpb.SignedBeaconBlockBellatrix {
	return HydrateSignedBeaconBlockBellatrix(&zondpb.SignedBeaconBlockBellatrix{})
}

// NewBlindedBeaconBlockBellatrix creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockBellatrix() *zondpb.SignedBlindedBeaconBlockBellatrix {
	return HydrateSignedBlindedBeaconBlockBellatrix(&zondpb.SignedBlindedBeaconBlockBellatrix{})
}

// NewBlindedBeaconBlockBellatrixV2 creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockBellatrixV2() *v2.SignedBlindedBeaconBlockBellatrix {
	return HydrateV2SignedBlindedBeaconBlockBellatrix(&v2.SignedBlindedBeaconBlockBellatrix{})
}

// NewBeaconBlockCapella creates a beacon block with minimum marshalable fields.
func NewBeaconBlockCapella() *zondpb.SignedBeaconBlockCapella {
	return HydrateSignedBeaconBlockCapella(&zondpb.SignedBeaconBlockCapella{})
}

// NewBlindedBeaconBlockCapella creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockCapella() *zondpb.SignedBlindedBeaconBlockCapella {
	return HydrateSignedBlindedBeaconBlockCapella(&zondpb.SignedBlindedBeaconBlockCapella{})
}

// NewBlindedBeaconBlockCapellaV2 creates a blinded beacon block with minimum marshalable fields.
func NewBlindedBeaconBlockCapellaV2() *v2.SignedBlindedBeaconBlockCapella {
	return HydrateV2SignedBlindedBeaconBlockCapella(&v2.SignedBlindedBeaconBlockCapella{})
}
