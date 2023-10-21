package beacon_api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type beaconApiNodeClient struct {
	fallbackClient  iface.NodeClient
	jsonRestHandler jsonRestHandler
	genesisProvider genesisProvider
}

func (c *beaconApiNodeClient) GetSyncStatus(ctx context.Context, _ *empty.Empty) (*zondpb.SyncStatus, error) {
	syncingResponse := apimiddleware.SyncingResponseJson{}
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, "/zond/v1/node/syncing", &syncingResponse); err != nil {
		return nil, errors.Wrap(err, "failed to get sync status")
	}

	if syncingResponse.Data == nil {
		return nil, errors.New("syncing data is nil")
	}

	return &zondpb.SyncStatus{
		Syncing: syncingResponse.Data.IsSyncing,
	}, nil
}

func (c *beaconApiNodeClient) GetGenesis(ctx context.Context, _ *empty.Empty) (*zondpb.Genesis, error) {
	genesisJson, _, err := c.genesisProvider.GetGenesis(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get genesis")
	}

	genesisValidatorRoot, err := hexutil.Decode(genesisJson.GenesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode genesis validator root `%s`", genesisJson.GenesisValidatorsRoot)
	}

	genesisTime, err := strconv.ParseInt(genesisJson.GenesisTime, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse genesis time `%s`", genesisJson.GenesisTime)
	}

	depositContractJson := apimiddleware.DepositContractResponseJson{}
	if _, err = c.jsonRestHandler.GetRestJsonResponse(ctx, "/zond/v1/config/deposit_contract", &depositContractJson); err != nil {
		return nil, errors.Wrapf(err, "failed to query deposit contract information")
	}

	if depositContractJson.Data == nil {
		return nil, errors.New("deposit contract data is nil")
	}

	depositContactAddress, err := hexutil.Decode(depositContractJson.Data.Address)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode deposit contract address `%s`", depositContractJson.Data.Address)
	}

	return &zondpb.Genesis{
		GenesisTime: &timestamppb.Timestamp{
			Seconds: genesisTime,
		},
		DepositContractAddress: depositContactAddress,
		GenesisValidatorsRoot:  genesisValidatorRoot,
	}, nil
}

func (c *beaconApiNodeClient) GetVersion(ctx context.Context, in *empty.Empty) (*zondpb.Version, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.GetVersion(ctx, in)
	}

	// TODO: Implement me
	panic("beaconApiNodeClient.GetVersion is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiNodeClientWithFallback.")
}

func (c *beaconApiNodeClient) ListPeers(ctx context.Context, in *empty.Empty) (*zondpb.Peers, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.ListPeers(ctx, in)
	}

	// TODO: Implement me
	panic("beaconApiNodeClient.ListPeers is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiNodeClientWithFallback.")
}

func NewNodeClientWithFallback(host string, timeout time.Duration, fallbackClient iface.NodeClient) iface.NodeClient {
	jsonRestHandler := beaconApiJsonRestHandler{
		httpClient: http.Client{Timeout: timeout},
		host:       host,
	}

	return &beaconApiNodeClient{
		jsonRestHandler: jsonRestHandler,
		fallbackClient:  fallbackClient,
		genesisProvider: beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler},
	}
}
