package migration

import (
	"github.com/pkg/errors"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// ZondToV1Alpha1SignedBlock converts a v1 SignedBeaconBlockZond proto to a v1alpha1 proto.
func ZondToV1Alpha1SignedBlock(zondBlk *qrlpb.SignedBeaconBlockZond) (*qrysmpb.SignedBeaconBlockZond, error) {
	marshaledBlk, err := proto.Marshal(zondBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &qrysmpb.SignedBeaconBlockZond{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	// Proposer slashing headers have different protobuf field numbers in v1alpha1.
	// Map them explicitly and preserve the independent storage of the conversion.
	for i, slashing := range zondBlk.GetMessage().GetBody().GetProposerSlashings() {
		v1alpha1Block.Block.Body.ProposerSlashings[i] = proto.Clone(V1ProposerSlashingToV1Alpha1(slashing)).(*qrysmpb.ProposerSlashing)
	}
	return v1alpha1Block, nil
}

// BlindedZondToV1Alpha1SignedBlock converts a v1 SignedBlindedBeaconBlockZond proto to a v1alpha1 proto.
func BlindedZondToV1Alpha1SignedBlock(zondBlk *qrlpb.SignedBlindedBeaconBlockZond) (*qrysmpb.SignedBlindedBeaconBlockZond, error) {
	marshaledBlk, err := proto.Marshal(zondBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &qrysmpb.SignedBlindedBeaconBlockZond{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	// Proposer slashing headers have different protobuf field numbers in v1alpha1.
	for i, slashing := range zondBlk.GetMessage().GetBody().GetProposerSlashings() {
		v1alpha1Block.Block.Body.ProposerSlashings[i] = proto.Clone(V1ProposerSlashingToV1Alpha1(slashing)).(*qrysmpb.ProposerSlashing)
	}
	return v1alpha1Block, nil
}
