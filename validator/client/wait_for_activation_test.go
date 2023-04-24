package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cyyber/qrysm/v4/crypto/dilithium"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/mock"
	"github.com/cyyber/qrysm/v4/testing/require"
	validatormock "github.com/cyyber/qrysm/v4/testing/validator-mock"
	walletMock "github.com/cyyber/qrysm/v4/validator/accounts/testing"
	"github.com/cyyber/qrysm/v4/validator/keymanager/derived"
	constant "github.com/cyyber/qrysm/v4/validator/testing"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/tyler-smith/go-bip39"
	util "github.com/wealdtech/go-eth2-util"
)

func TestWaitActivation_ContextCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		beaconClient:    beaconClient,
	}
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)

	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, nil)
	clientStream.EXPECT().Recv().Return(
		&ethpb.ValidatorActivationResponse{},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorContains(t, cancelledCtx, v.WaitForActivation(ctx, nil))
}

func TestWaitActivation_StreamSetupFails_AttemptsToReconnect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		beaconClient:    beaconClient,
	}
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, errors.New("failed stream")).Return(clientStream, nil)
	beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)
	resp := generateMockStatusResponse([][]byte{pubKey[:]})
	resp.Statuses[0].Status.Status = ethpb.ValidatorStatus_ACTIVE
	clientStream.EXPECT().Recv().Return(resp, nil)
	assert.NoError(t, v.WaitForActivation(context.Background(), nil))
}

func TestWaitForActivation_ReceiveErrorFromStream_AttemptsReconnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		beaconClient:    beaconClient,
	}
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, nil)
	beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)
	// A stream fails the first time, but succeeds the second time.
	resp := generateMockStatusResponse([][]byte{pubKey[:]})
	resp.Statuses[0].Status.Status = ethpb.ValidatorStatus_ACTIVE
	clientStream.EXPECT().Recv().Return(
		nil,
		errors.New("fails"),
	).Return(resp, nil)
	assert.NoError(t, v.WaitForActivation(context.Background(), nil))
}

func TestWaitActivation_LogsActivationEpochOK(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		genesisTime:     1,
		beaconClient:    beaconClient,
	}
	resp := generateMockStatusResponse([][]byte{pubKey[:]})
	resp.Statuses[0].Status.Status = ethpb.ValidatorStatus_ACTIVE
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, nil)
	beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)
	clientStream.EXPECT().Recv().Return(
		resp,
		nil,
	)
	assert.NoError(t, v.WaitForActivation(context.Background(), nil), "Could not wait for activation")
	assert.LogsContain(t, hook, "Validator activated")
}

func TestWaitForActivation_Exiting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		beaconClient:    beaconClient,
	}
	resp := generateMockStatusResponse([][]byte{pubKey[:]})
	resp.Statuses[0].Status.Status = ethpb.ValidatorStatus_EXITING
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, nil)
	beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)
	clientStream.EXPECT().Recv().Return(
		resp,
		nil,
	)
	assert.NoError(t, v.WaitForActivation(context.Background(), nil))
}

func TestWaitForActivation_RefetchKeys(t *testing.T) {
	originalPeriod := keyRefetchPeriod
	defer func() {
		keyRefetchPeriod = originalPeriod
	}()
	keyRefetchPeriod = 1 * time.Second

	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
	privKey, err := dilithium.RandKey()
	require.NoError(t, err)
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], privKey.PublicKey().Marshal())
	km := &mockKeymanager{
		keysMap: map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey{
			pubKey: privKey,
		},
		fetchNoKeys: true,
	}
	v := validator{
		validatorClient: validatorClient,
		keyManager:      km,
		beaconClient:    beaconClient,
	}
	resp := generateMockStatusResponse([][]byte{pubKey[:]})
	resp.Statuses[0].Status.Status = ethpb.ValidatorStatus_ACTIVE
	clientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
	validatorClient.EXPECT().WaitForActivation(
		gomock.Any(),
		&ethpb.ValidatorActivationRequest{
			PublicKeys: [][]byte{pubKey[:]},
		},
	).Return(clientStream, nil)
	beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil)
	clientStream.EXPECT().Recv().Return(
		resp,
		nil)
	assert.NoError(t, v.internalWaitForActivation(context.Background(), make(chan [][dilithium2.CryptoPublicKeyBytes]byte)), "Could not wait for activation")
	assert.LogsContain(t, hook, msgNoKeysFetched)
	assert.LogsContain(t, hook, "Validator activated")
}

// Regression test for a scenario where you start with an inactive key and then import an active key.
func TestWaitForActivation_AccountsChanged(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Imported keymanager", func(t *testing.T) {
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
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
		v := validator{
			validatorClient: validatorClient,
			keyManager:      km,
			beaconClient:    beaconClient,
		}
		inactiveResp := generateMockStatusResponse([][]byte{inactivePubKey[:]})
		inactiveResp.Statuses[0].Status.Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		inactiveClientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
		validatorClient.EXPECT().WaitForActivation(
			gomock.Any(),
			&ethpb.ValidatorActivationRequest{
				PublicKeys: [][]byte{inactivePubKey[:]},
			},
		).Return(inactiveClientStream, nil)
		beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil).AnyTimes()
		inactiveClientStream.EXPECT().Recv().Return(
			inactiveResp,
			nil,
		).AnyTimes()

		activeResp := generateMockStatusResponse([][]byte{inactivePubKey[:], activePubKey[:]})
		activeResp.Statuses[0].Status.Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status.Status = ethpb.ValidatorStatus_ACTIVE
		activeClientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
		validatorClient.EXPECT().WaitForActivation(
			gomock.Any(),
			&ethpb.ValidatorActivationRequest{
				PublicKeys: [][]byte{inactivePubKey[:], activePubKey[:]},
			},
		).Return(activeClientStream, nil)
		activeClientStream.EXPECT().Recv().Return(
			activeResp,
			nil,
		)

		go func() {
			// We add the active key into the keymanager and simulate a key refresh.
			time.Sleep(time.Second * 1)
			km.keysMap[activePubKey] = activePrivKey
			km.SimulateAccountChanges(make([][dilithium2.CryptoPublicKeyBytes]byte, 0))
		}()

		assert.NoError(t, v.WaitForActivation(context.Background(), nil))
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})

	t.Run("Derived keymanager", func(t *testing.T) {
		seed := bip39.NewSeed(constant.TestMnemonic, "")
		inactivePrivKey, err :=
			util.PrivateKeyFromSeedAndPath(seed, fmt.Sprintf(derived.ValidatingKeyDerivationPathTemplate, 0))
		require.NoError(t, err)
		var inactivePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(inactivePubKey[:], inactivePrivKey.PublicKey().Marshal())
		activePrivKey, err :=
			util.PrivateKeyFromSeedAndPath(seed, fmt.Sprintf(derived.ValidatingKeyDerivationPathTemplate, 1))
		require.NoError(t, err)
		var activePubKey [dilithium2.CryptoPublicKeyBytes]byte
		copy(activePubKey[:], activePrivKey.PublicKey().Marshal())
		wallet := &walletMock.Wallet{
			Files:            make(map[string]map[string][]byte),
			AccountPasswords: make(map[string]string),
			WalletPassword:   "secretPassw0rd$1999",
		}
		ctx := context.Background()
		km, err := derived.NewKeymanager(ctx, &derived.SetupConfig{
			Wallet:           wallet,
			ListenForChanges: true,
		})
		require.NoError(t, err)
		err = km.RecoverAccountsFromMnemonic(ctx, constant.TestMnemonic, derived.DefaultMnemonicLanguage, "", 1)
		require.NoError(t, err)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		beaconClient := validatormock.NewMockBeaconChainClient(ctrl)
		v := validator{
			validatorClient: validatorClient,
			keyManager:      km,
			genesisTime:     1,
			beaconClient:    beaconClient,
		}

		inactiveResp := generateMockStatusResponse([][]byte{inactivePubKey[:]})
		inactiveResp.Statuses[0].Status.Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		inactiveClientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
		validatorClient.EXPECT().WaitForActivation(
			gomock.Any(),
			&ethpb.ValidatorActivationRequest{
				PublicKeys: [][]byte{inactivePubKey[:]},
			},
		).Return(inactiveClientStream, nil)
		beaconClient.EXPECT().ListValidators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{}, nil).AnyTimes()
		inactiveClientStream.EXPECT().Recv().Return(
			inactiveResp,
			nil,
		).AnyTimes()

		activeResp := generateMockStatusResponse([][]byte{inactivePubKey[:], activePubKey[:]})
		activeResp.Statuses[0].Status.Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status.Status = ethpb.ValidatorStatus_ACTIVE
		activeClientStream := mock.NewMockBeaconNodeValidator_WaitForActivationClient(ctrl)
		validatorClient.EXPECT().WaitForActivation(
			gomock.Any(),
			&ethpb.ValidatorActivationRequest{
				PublicKeys: [][]byte{inactivePubKey[:], activePubKey[:]},
			},
		).Return(activeClientStream, nil)
		activeClientStream.EXPECT().Recv().Return(
			activeResp,
			nil,
		)

		channel := make(chan [][dilithium2.CryptoPublicKeyBytes]byte)
		go func() {
			// We add the active key into the keymanager and simulate a key refresh.
			time.Sleep(time.Second * 1)
			err = km.RecoverAccountsFromMnemonic(ctx, constant.TestMnemonic, derived.DefaultMnemonicLanguage, "", 2)
			require.NoError(t, err)
			channel <- [][dilithium2.CryptoPublicKeyBytes]byte{}
		}()

		assert.NoError(t, v.internalWaitForActivation(context.Background(), channel))
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})
}
