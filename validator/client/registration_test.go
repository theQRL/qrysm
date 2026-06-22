package client

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSubmitValidatorRegistrations(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	ctx := context.Background()
	require.NoError(t, nil, SubmitValidatorRegistrations(ctx, m.validatorClient, []*qrysmpb.SignedValidatorRegistrationV1{}))

	reg := &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("fee"), fieldparams.FeeRecipientLength),
		GasLimit:     123456,
		Timestamp:    uint64(time.Now().Unix()),
		Pubkey:       validatorKey.PublicKey().Marshal(),
	}

	m.validatorClient.EXPECT().
		SubmitValidatorRegistrations(gomock.Any(), &qrysmpb.SignedValidatorRegistrationsV1{
			Messages: []*qrysmpb.SignedValidatorRegistrationV1{
				{Message: reg,
					Signature: params.BeaconConfig().ZeroHash[:]},
			},
		}).
		Return(nil, nil)
	require.NoError(t, nil, SubmitValidatorRegistrations(ctx, m.validatorClient, []*qrysmpb.SignedValidatorRegistrationV1{
		{Message: reg,
			Signature: params.BeaconConfig().ZeroHash[:]},
	}))
}

func TestSubmitValidatorRegistration_CantSign(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	ctx := context.Background()
	reg := &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("fee"), fieldparams.FeeRecipientLength),
		GasLimit:     123456,
		Timestamp:    uint64(time.Now().Unix()),
		Pubkey:       validatorKey.PublicKey().Marshal(),
	}

	m.validatorClient.EXPECT().
		SubmitValidatorRegistrations(gomock.Any(), &qrysmpb.SignedValidatorRegistrationsV1{
			Messages: []*qrysmpb.SignedValidatorRegistrationV1{
				{Message: reg,
					Signature: params.BeaconConfig().ZeroHash[:]},
			},
		}).
		Return(nil, errors.New("could not sign"))
	require.ErrorContains(t, "could not sign", SubmitValidatorRegistrations(ctx, m.validatorClient, []*qrysmpb.SignedValidatorRegistrationV1{
		{Message: reg,
			Signature: params.BeaconConfig().ZeroHash[:]},
	}))
}

func Test_signValidatorRegistration(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	ctx := context.Background()
	reg := &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("fee"), fieldparams.FeeRecipientLength),
		GasLimit:     123456,
		Timestamp:    uint64(time.Now().Unix()),
		Pubkey:       validatorKey.PublicKey().Marshal(),
	}
	_, err := signValidatorRegistration(ctx, m.signfunc, reg)
	require.NoError(t, err)

}

func TestValidator_SignValidatorRegistrationRequest(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()
	ctx := context.Background()
	byteval, err := hexutil.DecodeQ("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000878705ba3f8bc32fcf7f4caa1a35e72af65cf766")
	require.NoError(t, err)
	tests := []struct {
		name            string
		arg             *qrysmpb.ValidatorRegistrationV1
		validatorSetter func(t *testing.T) *validator
		isCached        bool
		err             string
	}{
		{
			name: " Happy Path cached",
			arg: &qrysmpb.ValidatorRegistrationV1{
				Pubkey:       validatorKey.PublicKey().Marshal(),
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				GasLimit:     30000000,
				Timestamp:    uint64(time.Now().Unix()),
			},
			validatorSetter: func(t *testing.T) *validator {
				v := validator{
					pubkeyToValidatorIndex:       make(map[[field_params.MLDSA87PubkeyLength]byte]primitives.ValidatorIndex),
					signedValidatorRegistrations: make(map[[field_params.MLDSA87PubkeyLength]byte]*qrysmpb.SignedValidatorRegistrationV1),
					genesisTime:                  0,
				}
				v.signedValidatorRegistrations[bytesutil.ToBytes2592(validatorKey.PublicKey().Marshal())] = &qrysmpb.SignedValidatorRegistrationV1{
					Message: &qrysmpb.ValidatorRegistrationV1{
						Pubkey:       validatorKey.PublicKey().Marshal(),
						GasLimit:     30000000,
						FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
						Timestamp:    uint64(time.Now().Unix()),
					},
					Signature: make([]byte, 0),
				}
				return &v
			},
			isCached: true,
		},
		{
			name: " Happy Path not cached gas updated",
			arg: &qrysmpb.ValidatorRegistrationV1{
				Pubkey:       validatorKey.PublicKey().Marshal(),
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				GasLimit:     30000000,
				Timestamp:    uint64(time.Now().Unix()),
			},
			validatorSetter: func(t *testing.T) *validator {
				v := validator{
					pubkeyToValidatorIndex:       make(map[[field_params.MLDSA87PubkeyLength]byte]primitives.ValidatorIndex),
					signedValidatorRegistrations: make(map[[field_params.MLDSA87PubkeyLength]byte]*qrysmpb.SignedValidatorRegistrationV1),
					genesisTime:                  0,
				}
				v.signedValidatorRegistrations[bytesutil.ToBytes2592(validatorKey.PublicKey().Marshal())] = &qrysmpb.SignedValidatorRegistrationV1{
					Message: &qrysmpb.ValidatorRegistrationV1{
						Pubkey:       validatorKey.PublicKey().Marshal(),
						GasLimit:     35000000,
						FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
						Timestamp:    uint64(time.Now().Unix() - 1),
					},
					Signature: make([]byte, 0),
				}
				return &v
			},
			isCached: false,
		},
		{
			name: " Happy Path not cached feerecipient updated",
			arg: &qrysmpb.ValidatorRegistrationV1{
				Pubkey:       validatorKey.PublicKey().Marshal(),
				FeeRecipient: byteval,
				GasLimit:     30000000,
				Timestamp:    uint64(time.Now().Unix()),
			},
			validatorSetter: func(t *testing.T) *validator {
				v := validator{
					pubkeyToValidatorIndex:       make(map[[field_params.MLDSA87PubkeyLength]byte]primitives.ValidatorIndex),
					signedValidatorRegistrations: make(map[[field_params.MLDSA87PubkeyLength]byte]*qrysmpb.SignedValidatorRegistrationV1),
					genesisTime:                  0,
				}
				v.signedValidatorRegistrations[bytesutil.ToBytes2592(validatorKey.PublicKey().Marshal())] = &qrysmpb.SignedValidatorRegistrationV1{
					Message: &qrysmpb.ValidatorRegistrationV1{
						Pubkey:       validatorKey.PublicKey().Marshal(),
						GasLimit:     30000000,
						FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
						Timestamp:    uint64(time.Now().Unix() - 1),
					},
					Signature: make([]byte, 0),
				}
				return &v
			},
			isCached: false,
		},
		{
			name: " Happy Path not cached first Entry",
			arg: &qrysmpb.ValidatorRegistrationV1{
				Pubkey:       validatorKey.PublicKey().Marshal(),
				FeeRecipient: byteval,
				GasLimit:     30000000,
				Timestamp:    uint64(time.Now().Unix()),
			},
			validatorSetter: func(t *testing.T) *validator {
				v := validator{
					pubkeyToValidatorIndex:       make(map[[field_params.MLDSA87PubkeyLength]byte]primitives.ValidatorIndex),
					signedValidatorRegistrations: make(map[[field_params.MLDSA87PubkeyLength]byte]*qrysmpb.SignedValidatorRegistrationV1),
					genesisTime:                  0,
				}
				return &v
			},
			isCached: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.validatorSetter(t)

			startingReq, ok := v.signedValidatorRegistrations[bytesutil.ToBytes2592(tt.arg.Pubkey)]

			got, err := v.SignValidatorRegistrationRequest(ctx, m.signfunc, tt.arg)
			require.NoError(t, err)
			if tt.isCached {
				require.DeepEqual(t, got, v.signedValidatorRegistrations[bytesutil.ToBytes2592(tt.arg.Pubkey)])
			} else {
				if ok {
					require.NotEqual(t, got.Message.Timestamp, startingReq.Message.Timestamp)
				}
				require.Equal(t, got.Message.Timestamp, tt.arg.Timestamp)
				require.Equal(t, got.Message.GasLimit, tt.arg.GasLimit)
				require.Equal(t, hexutil.Encode(got.Message.FeeRecipient), hexutil.Encode(tt.arg.FeeRecipient))
				require.DeepEqual(t, got, v.signedValidatorRegistrations[bytesutil.ToBytes2592(tt.arg.Pubkey)])
			}
		})
	}
}

func TestValidator_SignValidatorRegistrationRequest_ConcurrentSamePubkeyUsesCachedRegistration(t *testing.T) {
	_, _, validatorKey, finish := setup(t)
	defer finish()

	ctx := context.Background()
	reg := &qrysmpb.ValidatorRegistrationV1{
		Pubkey:       validatorKey.PublicKey().Marshal(),
		FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
		GasLimit:     30000000,
		Timestamp:    uint64(time.Now().Unix()),
	}

	v := &validator{
		pubkeyToValidatorIndex:       make(map[[field_params.MLDSA87PubkeyLength]byte]primitives.ValidatorIndex),
		signedValidatorRegistrations: make(map[[field_params.MLDSA87PubkeyLength]byte]*qrysmpb.SignedValidatorRegistrationV1),
	}

	const concurrentCalls = 8
	start := make(chan struct{})
	var signCalls atomic.Int32
	var wg sync.WaitGroup
	results := make(chan *qrysmpb.SignedValidatorRegistrationV1, concurrentCalls)
	errs := make(chan error, concurrentCalls)

	signer := func(ctx context.Context, req *validatorpb.SignRequest) (ml_dsa_87.Signature, error) {
		signCalls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return mockSignature{}, nil
	}

	for range concurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := v.SignValidatorRegistrationRequest(ctx, signer, reg)
			if err != nil {
				errs <- err
				return
			}
			results <- got
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), signCalls.Load())
	require.Equal(t, 1, len(v.signedValidatorRegistrations))

	var first *qrysmpb.SignedValidatorRegistrationV1
	for got := range results {
		if first == nil {
			first = got
			continue
		}
		require.DeepEqual(t, first, got)
	}
	require.NotNil(t, first)
}
