package kv

import (
	"context"
	"testing"

	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/config/params"
	validatorServiceConfig "github.com/theQRL/qrysm/v4/config/validator/service"
	"github.com/theQRL/qrysm/v4/consensus-types/validator"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestStore_ProposerSettings_ReadAndWrite(t *testing.T) {
	t.Run("save to db in full", func(t *testing.T) {
		ctx := context.Background()
		db := setupDB(t, [][dilithium2.CryptoPublicKeyBytes]byte{})
		key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
		require.NoError(t, err)
		settings := &validatorServiceConfig.ProposerSettings{
			ProposeConfig: map[[dilithium2.CryptoPublicKeyBytes]byte]*validatorServiceConfig.ProposerOption{
				bytesutil.ToBytes2592(key1): {
					FeeRecipientConfig: &validatorServiceConfig.FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &validatorServiceConfig.BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
					},
				},
			},
			DefaultConfig: &validatorServiceConfig.ProposerOption{
				FeeRecipientConfig: &validatorServiceConfig.FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
				},
				BuilderConfig: &validatorServiceConfig.BuilderConfig{
					Enabled:  false,
					GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				},
			},
		}
		err = db.SaveProposerSettings(ctx, settings)
		require.NoError(t, err)

		dbSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, settings, dbSettings)
	})
	t.Run("update default settings then update at specific key", func(t *testing.T) {
		ctx := context.Background()
		db := setupDB(t, [][dilithium2.CryptoPublicKeyBytes]byte{})
		key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
		require.NoError(t, err)
		settings := &validatorServiceConfig.ProposerSettings{
			DefaultConfig: &validatorServiceConfig.ProposerOption{
				FeeRecipientConfig: &validatorServiceConfig.FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
				},
				BuilderConfig: &validatorServiceConfig.BuilderConfig{
					Enabled:  false,
					GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				},
			},
		}
		err = db.SaveProposerSettings(ctx, settings)
		require.NoError(t, err)
		upatedDefault := &validatorServiceConfig.ProposerOption{
			FeeRecipientConfig: &validatorServiceConfig.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x9995733c5af9B61374A128e6F85f553aF09ff89B"),
			},
			BuilderConfig: &validatorServiceConfig.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
			},
		}
		err = db.UpdateProposerSettingsDefault(ctx, upatedDefault)
		require.NoError(t, err)

		dbSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbSettings)
		require.DeepEqual(t, dbSettings.DefaultConfig, upatedDefault)
		option := &validatorServiceConfig.ProposerOption{
			FeeRecipientConfig: &validatorServiceConfig.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
			},
			BuilderConfig: &validatorServiceConfig.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(40000000),
			},
		}
		err = db.UpdateProposerSettingsForPubkey(ctx, bytesutil.ToBytes2592(key1), option)
		require.NoError(t, err)

		newSettings, err := db.ProposerSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, newSettings)
		require.DeepEqual(t, newSettings.DefaultConfig, upatedDefault)
		op, ok := newSettings.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, ok, true)
		require.DeepEqual(t, op, option)
	})
}
