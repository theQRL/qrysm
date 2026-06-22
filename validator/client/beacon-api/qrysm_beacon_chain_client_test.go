package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	gatewaymiddleware "github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrysm/validator"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	validator_mock "github.com/theQRL/qrysm/testing/validator-mock"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/protobuf/types/known/emptypb"
)

func Test_qrysmBeaconChainClient_GetValidatorPerformance(t *testing.T) {
	publicKeys := [][2592]byte{
		bytesutil.ToBytes2592([]byte{1}),
		bytesutil.ToBytes2592([]byte{2}),
		bytesutil.ToBytes2592([]byte{3}),
	}

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	request, err := json.Marshal(validator.ValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)

	wantResponse := &validator.ValidatorPerformanceResponse{}
	want := &qrysmpb.ValidatorPerformanceResponse{}

	nodeClient := validator_mock.NewMockNodeClient(ctrl)
	nodeClient.EXPECT().GetVersion(ctx, &emptypb.Empty{}).Return(
		&qrysmpb.Version{Version: "qrysm/v0.0.1"}, nil,
	)

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		getValidatorPerformanceEndpoint,
		nil,
		bytes.NewBuffer(request),
		wantResponse,
	).Return(&gatewaymiddleware.DefaultErrorJson{}, nil)

	c := qrysmBeaconChainClient{
		nodeClient:      nodeClient,
		jsonRestHandler: jsonRestHandler,
	}

	got, err := c.GetValidatorPerformance(ctx, &qrysmpb.ValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)
	require.DeepEqual(t, want.PublicKeys, got.PublicKeys)
}

func Test_qrysmBeaconChainClient_GetValidatorPerformance_NonQrysmNode(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nodeClient := validator_mock.NewMockNodeClient(ctrl)
	nodeClient.EXPECT().GetVersion(ctx, &emptypb.Empty{}).Return(
		&qrysmpb.Version{Version: "lighthouse/v0.0.1"}, nil,
	)

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)

	c := qrysmBeaconChainClient{
		nodeClient:      nodeClient,
		jsonRestHandler: jsonRestHandler,
	}

	_, err := c.GetValidatorPerformance(ctx, &qrysmpb.ValidatorPerformanceRequest{})
	assert.ErrorContains(t, "endpoint not supported", err)
	require.ErrorIs(t, err, iface.ErrNotSupported)
}
