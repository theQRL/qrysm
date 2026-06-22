package util

import (
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// HydrateSyncCommittee hydrates the provided sync committee message.
func HydrateSyncCommittee(s *qrysmpb.SyncCommitteeMessage) *qrysmpb.SyncCommitteeMessage {
	if s.Signature == nil {
		s.Signature = make([]byte, 4627)
	}
	if s.BlockRoot == nil {
		s.BlockRoot = make([]byte, fieldparams.RootLength)
	}
	return s
}

// ConvertToCommittee takes a list of pubkeys and returns a SyncCommittee with
// these keys as members. Some keys may appear repeated
func ConvertToCommittee(inputKeys [][]byte) *qrysmpb.SyncCommittee {
	var pubKeys [][]byte
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSize; i++ {
		if i < uint64(len(inputKeys)) {
			pubKeys = append(pubKeys, bytesutil.PadTo(inputKeys[i], field_params.MLDSA87PubkeyLength))
		} else {
			pubKeys = append(pubKeys, bytesutil.PadTo([]byte{}, field_params.MLDSA87PubkeyLength))
		}
	}

	return &qrysmpb.SyncCommittee{
		Pubkeys: pubKeys,
	}
}
