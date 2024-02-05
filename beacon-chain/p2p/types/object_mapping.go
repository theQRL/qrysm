package types

import (
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/wrapper"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/metadata"
)

func init() {
	// Initialize data maps.
	InitializeDataMaps()
}

// This file provides a mapping of fork version to the respective data type. This is
// to allow any service to appropriately use the correct data type with a provided
// fork-version.

var (
	// BlockMap maps the fork-version to the underlying data type for that
	// particular fork period.
	BlockMap map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error)
	// MetaDataMap maps the fork-version to the underlying data type for that
	// particular fork period.
	MetaDataMap map[[4]byte]func() metadata.Metadata
)

// InitializeDataMaps initializes all the relevant object maps. This function is called to
// reset maps and reinitialize them.
func InitializeDataMaps() {
	// Reset our block map.
	BlockMap = map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&zondpb.SignedBeaconBlock{Block: &zondpb.BeaconBlock{Body: &zondpb.BeaconBlockBody{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&zondpb.SignedBeaconBlockAltair{Block: &zondpb.BeaconBlockAltair{Body: &zondpb.BeaconBlockBodyAltair{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&zondpb.SignedBeaconBlockBellatrix{Block: &zondpb.BeaconBlockBellatrix{Body: &zondpb.BeaconBlockBodyBellatrix{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{}}},
			)
		},
	}

	// Reset our metadata map.
	MetaDataMap = map[[4]byte]func() metadata.Metadata{
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() metadata.Metadata {
			return wrapper.WrappedMetadataV0(&zondpb.MetaDataV0{})
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() metadata.Metadata {
			return wrapper.WrappedMetadataV1(&zondpb.MetaDataV1{})
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() metadata.Metadata {
			return wrapper.WrappedMetadataV1(&zondpb.MetaDataV1{})
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() metadata.Metadata {
			return wrapper.WrappedMetadataV1(&zondpb.MetaDataV1{})
		},
	}
}
