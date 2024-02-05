package migration

import (
	"github.com/pkg/errors"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbv2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	"google.golang.org/protobuf/proto"
)

// AltairToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockAltair proto to a v1alpha1 proto.
func AltairToV1Alpha1SignedBlock(altairBlk *zondpbv2.SignedBeaconBlockAltair) (*zondpbalpha.SignedBeaconBlockAltair, error) {
	marshaledBlk, err := proto.Marshal(altairBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockAltair{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BellatrixToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockBellatrix proto to a v1alpha1 proto.
func BellatrixToV1Alpha1SignedBlock(bellatrixBlk *zondpbv2.SignedBeaconBlockBellatrix) (*zondpbalpha.SignedBeaconBlockBellatrix, error) {
	marshaledBlk, err := proto.Marshal(bellatrixBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockBellatrix{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// CapellaToV1Alpha1SignedBlock converts a v2 SignedBeaconBlockCapella proto to a v1alpha1 proto.
func CapellaToV1Alpha1SignedBlock(capellaBlk *zondpbv2.SignedBeaconBlockCapella) (*zondpbalpha.SignedBeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(capellaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BlindedBellatrixToV1Alpha1SignedBlock converts a v2 SignedBlindedBeaconBlockBellatrix proto to a v1alpha1 proto.
func BlindedBellatrixToV1Alpha1SignedBlock(bellatrixBlk *zondpbv2.SignedBlindedBeaconBlockBellatrix) (*zondpbalpha.SignedBlindedBeaconBlockBellatrix, error) {
	marshaledBlk, err := proto.Marshal(bellatrixBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBlindedBeaconBlockBellatrix{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// BlindedCapellaToV1Alpha1SignedBlock converts a v2 SignedBlindedBeaconBlockCapella proto to a v1alpha1 proto.
func BlindedCapellaToV1Alpha1SignedBlock(capellaBlk *zondpbv2.SignedBlindedBeaconBlockCapella) (*zondpbalpha.SignedBlindedBeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(capellaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBlindedBeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}
