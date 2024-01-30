package validator

import (
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/time/slots"
)

// setKzgCommitments sets the KZG commitment on the block.
// Return early if the block version is older than deneb or block slot has not passed deneb epoch.
// Depends on the blk is blind or not, set the KZG commitment from the corresponding bundle.
func setKzgCommitments(blk interfaces.SignedBeaconBlock, bundle *enginev1.BlobsBundle, blindBundle *enginev1.BlindedBlobsBundle) error {
	if blk.Version() < version.Deneb {
		return nil
	}
	slot := blk.Block().Slot()
	if slots.ToEpoch(slot) < params.BeaconConfig().DenebForkEpoch {
		return nil
	}

	if blk.IsBlinded() {
		if blindBundle == nil {
			return nil
		}
		return blk.SetBlobKzgCommitments(blindBundle.KzgCommitments)
	}

	if bundle == nil {
		return nil
	}
	return blk.SetBlobKzgCommitments(bundle.KzgCommitments)
}

// coverts a blobs bundle to a sidecar format.
func blobsBundleToSidecars(bundle *enginev1.BlobsBundle, blk interfaces.ReadOnlyBeaconBlock) ([]*zondpb.BlobSidecar, error) {
	if blk.Version() < version.Deneb {
		return nil, nil
	}
	if bundle == nil || len(bundle.KzgCommitments) == 0 {
		return nil, nil
	}
	r, err := blk.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	pr := blk.ParentRoot()

	sidecars := make([]*zondpb.BlobSidecar, len(bundle.Blobs))
	for i := 0; i < len(bundle.Blobs); i++ {
		sidecars[i] = &zondpb.BlobSidecar{
			BlockRoot:       r[:],
			Index:           uint64(i),
			Slot:            blk.Slot(),
			BlockParentRoot: pr[:],
			ProposerIndex:   blk.ProposerIndex(),
			Blob:            bundle.Blobs[i],
			KzgCommitment:   bundle.KzgCommitments[i],
			KzgProof:        bundle.Proofs[i],
		}
	}

	return sidecars, nil
}

// coverts a blinds blobs bundle to a sidecar format.
func blindBlobsBundleToSidecars(bundle *enginev1.BlindedBlobsBundle, blk interfaces.ReadOnlyBeaconBlock) ([]*zondpb.BlindedBlobSidecar, error) {
	if blk.Version() < version.Deneb {
		return nil, nil
	}
	if bundle == nil || len(bundle.KzgCommitments) == 0 {
		return nil, nil
	}
	r, err := blk.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	pr := blk.ParentRoot()

	sidecars := make([]*zondpb.BlindedBlobSidecar, len(bundle.BlobRoots))
	for i := 0; i < len(bundle.BlobRoots); i++ {
		sidecars[i] = &zondpb.BlindedBlobSidecar{
			BlockRoot:       r[:],
			Index:           uint64(i),
			Slot:            blk.Slot(),
			BlockParentRoot: pr[:],
			ProposerIndex:   blk.ProposerIndex(),
			BlobRoot:        bundle.BlobRoots[i],
			KzgCommitment:   bundle.KzgCommitments[i],
			KzgProof:        bundle.Proofs[i],
		}
	}

	return sidecars, nil
}
