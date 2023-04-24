package client

import (
	"context"
	"testing"

	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
	validatormock "github.com/cyyber/qrysm/v4/testing/validator-mock"
	"github.com/cyyber/qrysm/v4/validator/client/testutil"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

func TestValidator_HandleKeyReload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("active", func(t *testing.T) {
		hook := logTest.NewGlobal()

		inactivePrivKey, err := dilithium.RandKey()
		require.NoError(t, err)
		var inactivePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(inactivePubKey[:], inactivePrivKey.PublicKey().Marshal())
		activePrivKey, err := dilithium.RandKey()
		require.NoError(t, err)
		var activePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(activePubKey[:], activePrivKey.PublicKey().Marshal())
		km := &mockKeymanager{
			keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
				inactivePubKey: inactivePrivKey,
			},
		}
		client := validatormock.NewMockValidatorClient(ctrl)
		beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
		v := validator{
			validatorClient: client,
			keyManager:      km,
			genesisTime:     1,
			beaconClient:    beaconClient,
		}

		resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:], activePubKey[:]})
		resp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		resp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{inactivePubKey[:], activePubKey[:]},
			},
		).Return(resp, nil)
		beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)

		anyActive, err := v.HandleKeyReload(context.Background(), [][dilithium2.CryptoPublicKeyBytes]byte{inactivePubKey, activePubKey})
		require.NoError(t, err)
		assert.Equal(t, true, anyActive)
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})

	t.Run("no active", func(t *testing.T) {
		hook := logTest.NewGlobal()

		inactivePrivKey, err := dilithium.RandKey()
		require.NoError(t, err)
		var inactivePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(inactivePubKey[:], inactivePrivKey.PublicKey().Marshal())
		km := &mockKeymanager{
			keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
				inactivePubKey: inactivePrivKey,
			},
		}
		client := validatormock.NewMockValidatorClient(ctrl)
		beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
		v := validator{
			validatorClient: client,
			keyManager:      km,
			genesisTime:     1,
			beaconClient:    beaconClient,
		}

		resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:]})
		resp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{inactivePubKey[:]},
			},
		).Return(resp, nil)
		beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)

		anyActive, err := v.HandleKeyReload(context.Background(), [][dilithium2.CryptoPublicKeyBytes]byte{inactivePubKey})
		require.NoError(t, err)
		assert.Equal(t, false, anyActive)
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsDoNotContain(t, hook, "Validator activated")
	})

	t.Run("error when getting status", func(t *testing.T) {
		inactivePrivKey, err := dilithium.RandKey()
		require.NoError(t, err)
		var inactivePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(inactivePubKey[:], inactivePrivKey.PublicKey().Marshal())
		km := &mockKeymanager{
			keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
				inactivePubKey: inactivePrivKey,
			},
		}
		client := validatormock.NewMockValidatorClient(ctrl)
		v := validator{
			validatorClient: client,
			keyManager:      km,
			genesisTime:     1,
		}

		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{inactivePubKey[:]},
			},
		).Return(nil, errors.New("error"))

		_, err = v.HandleKeyReload(context.Background(), [][dilithium2.CryptoPublicKeyBytes]byte{inactivePubKey})
		assert.ErrorContains(t, "error", err)
	})
}
