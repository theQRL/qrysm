package beacon_api

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/grpc"
)

func (c beaconApiValidatorClient) waitForActivation(ctx context.Context, in *qrysmpb.ValidatorActivationRequest) (qrysmpb.BeaconNodeValidator_WaitForActivationClient, error) {
	return &waitForActivationClient{
		ctx:                        ctx,
		beaconApiValidatorClient:   c,
		ValidatorActivationRequest: in,
	}, nil
}

type waitForActivationClient struct {
	grpc.ClientStream
	ctx context.Context
	beaconApiValidatorClient
	*qrysmpb.ValidatorActivationRequest
	lastRecvTime time.Time
}

func computeWaitElements(now time.Time, lastRecvTime time.Time) (time.Duration, time.Time) {
	nextRecvTime := lastRecvTime.Add(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)

	if lastRecvTime.IsZero() {
		nextRecvTime = now
	}

	if nextRecvTime.Before(now) {
		return time.Duration(0), now
	}

	return nextRecvTime.Sub(now), nextRecvTime
}

func (c *waitForActivationClient) Recv() (*qrysmpb.ValidatorActivationResponse, error) {
	waitDuration, nextRecvTime := computeWaitElements(time.Now(), c.lastRecvTime)

	select {
	case <-time.After(waitDuration):
		c.lastRecvTime = nextRecvTime

		// Represents the target set of keys
		stringTargetPubKeysToPubKeys := make(map[string][]byte, len(c.ValidatorActivationRequest.PublicKeys))
		stringTargetPubKeys := make([]string, len(c.ValidatorActivationRequest.PublicKeys))

		// Represents the set of keys actually returned by the beacon node
		stringRetrievedPubKeys := make(map[string]struct{})

		// Contains all keys in targetPubKeys but not in retrievedPubKeys
		var missingPubKeys [][]byte

		statuses := []*qrysmpb.ValidatorActivationResponse_Status{}

		for index, publicKey := range c.ValidatorActivationRequest.PublicKeys {
			stringPubKey := hexutil.Encode(publicKey)
			stringTargetPubKeysToPubKeys[stringPubKey] = publicKey
			stringTargetPubKeys[index] = stringPubKey
		}

		stateValidators, err := c.stateValidatorsProvider.GetStateValidators(c.ctx, stringTargetPubKeys, nil, nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get state validators")
		}

		for _, data := range stateValidators.Data {
			pubkey, err := hexutil.Decode(data.Validator.Pubkey)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse validator public key")
			}

			stringRetrievedPubKeys[data.Validator.Pubkey] = struct{}{}

			index, err := strconv.ParseUint(data.Index, 10, 64)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse validator index")
			}

			validatorStatus, ok := beaconAPITogRPCValidatorStatus[data.Status]
			if !ok {
				return nil, errors.New("invalid validator status: " + data.Status)
			}

			statuses = append(statuses, &qrysmpb.ValidatorActivationResponse_Status{
				PublicKey: pubkey,
				Index:     primitives.ValidatorIndex(index),
				Status:    &qrysmpb.ValidatorStatusResponse{Status: validatorStatus},
			})
		}

		for stringTargetPubKey, targetPubKey := range stringTargetPubKeysToPubKeys {
			if _, ok := stringRetrievedPubKeys[stringTargetPubKey]; !ok {
				missingPubKeys = append(missingPubKeys, targetPubKey)
			}
		}

		for _, missingPubKey := range missingPubKeys {
			statuses = append(statuses, &qrysmpb.ValidatorActivationResponse_Status{
				PublicKey: missingPubKey,
				Index:     primitives.ValidatorIndex(^uint64(0)),
				Status:    &qrysmpb.ValidatorStatusResponse{Status: qrysmpb.ValidatorStatus_UNKNOWN_STATUS},
			})
		}

		return &qrysmpb.ValidatorActivationResponse{
			Statuses: statuses,
		}, nil
	case <-c.ctx.Done():
		return nil, errors.New("context canceled")
	}
}
