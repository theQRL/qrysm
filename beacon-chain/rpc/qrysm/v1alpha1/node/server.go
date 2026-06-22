// Package node defines a gRPC node service implementation, providing
// useful endpoints for checking a node's sync status, peer info,
// genesis data, and version information.
package node

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/sync"
	"github.com/theQRL/qrysm/io/logs"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server defines a server implementation of the gRPC Node service,
// providing RPC endpoints for verifying a beacon node's sync status, genesis and
// version information, and services the node implements and runs.
type Server struct {
	LogsStreamer              logs.Streamer
	SyncChecker               sync.Checker
	Server                    *grpc.Server
	BeaconDB                  db.ReadOnlyDatabase
	PeersFetcher              p2p.PeersProvider
	PeerManager               p2p.PeerManager
	GenesisTimeFetcher        blockchain.TimeFetcher
	GenesisFetcher            blockchain.GenesisFetcher
	ExecutionChainInfoFetcher execution.ChainInfoFetcher
	BeaconMonitoringHost      string
	BeaconMonitoringPort      int
}

// GetSyncStatus checks the current network sync status of the node.
func (ns *Server) GetSyncStatus(_ context.Context, _ *emptypb.Empty) (*qrysmpb.SyncStatus, error) {
	return &qrysmpb.SyncStatus{
		Syncing: ns.SyncChecker.Syncing(),
	}, nil
}

// GetGenesis fetches genesis chain information of QRL. Returns unix timestamp 0
// if a genesis time has yet to be determined.
func (ns *Server) GetGenesis(ctx context.Context, _ *emptypb.Empty) (*qrysmpb.Genesis, error) {
	contractAddr, err := ns.BeaconDB.DepositContractAddress(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve contract address from db: %v", err)
	}
	genesisTime := ns.GenesisTimeFetcher.GenesisTime()
	var defaultGenesisTime time.Time
	var gt *timestamppb.Timestamp
	if genesisTime == defaultGenesisTime {
		gt = timestamppb.New(time.Unix(0, 0))
	} else {
		gt = timestamppb.New(genesisTime)
	}

	genValRoot := ns.GenesisFetcher.GenesisValidatorsRoot()
	return &qrysmpb.Genesis{
		GenesisTime:            gt,
		DepositContractAddress: contractAddr,
		GenesisValidatorsRoot:  genValRoot[:],
	}, nil
}

// GetVersion checks the version information of the beacon node.
func (_ *Server) GetVersion(_ context.Context, _ *emptypb.Empty) (*qrysmpb.Version, error) {
	return &qrysmpb.Version{
		Version: version.Version(),
	}, nil
}

// ListImplementedServices lists the services implemented and enabled by this node.
//
// Any service not present in this list may return UNIMPLEMENTED or
// PERMISSION_DENIED. The server may also support fetching services by grpc
// reflection.
func (ns *Server) ListImplementedServices(_ context.Context, _ *emptypb.Empty) (*qrysmpb.ImplementedServices, error) {
	serviceInfo := ns.Server.GetServiceInfo()
	serviceNames := make([]string, 0, len(serviceInfo))
	for svc := range serviceInfo {
		serviceNames = append(serviceNames, svc)
	}
	sort.Strings(serviceNames)
	return &qrysmpb.ImplementedServices{
		Services: serviceNames,
	}, nil
}

// GetHost returns the p2p data on the current local and host peer.
func (ns *Server) GetHost(_ context.Context, _ *emptypb.Empty) (*qrysmpb.HostData, error) {
	var stringAddr []string
	for _, addr := range ns.PeerManager.Host().Addrs() {
		stringAddr = append(stringAddr, addr.String())
	}
	record := ns.PeerManager.QNR()
	qnr := ""
	var err error
	if record != nil {
		qnr, err = p2p.SerializeQNR(record)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Unable to serialize qnr: %v", err)
		}
	}

	return &qrysmpb.HostData{
		Addresses: stringAddr,
		PeerId:    ns.PeerManager.PeerID().String(),
		Qnr:       qnr,
	}, nil
}

// GetPeer returns the data known about the peer defined by the provided peer id.
func (ns *Server) GetPeer(_ context.Context, peerReq *qrysmpb.PeerRequest) (*qrysmpb.Peer, error) {
	pid, err := peer.Decode(peerReq.PeerId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Unable to parse provided peer id: %v", err)
	}
	addr, err := ns.PeersFetcher.Peers().Address(pid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Requested peer does not exist: %v", err)
	}
	dir, err := ns.PeersFetcher.Peers().Direction(pid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Requested peer does not exist: %v", err)
	}
	pbDirection := qrysmpb.PeerDirection_UNKNOWN
	switch dir {
	case network.DirInbound:
		pbDirection = qrysmpb.PeerDirection_INBOUND
	case network.DirOutbound:
		pbDirection = qrysmpb.PeerDirection_OUTBOUND
	}
	connState, err := ns.PeersFetcher.Peers().ConnectionState(pid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Requested peer does not exist: %v", err)
	}
	record, err := ns.PeersFetcher.Peers().QNR(pid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Requested peer does not exist: %v", err)
	}
	qnr := ""
	if record != nil {
		qnr, err = p2p.SerializeQNR(record)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Unable to serialize qnr: %v", err)
		}
	}
	return &qrysmpb.Peer{
		Address:         addr.String(),
		Direction:       pbDirection,
		ConnectionState: qrysmpb.ConnectionState(connState),
		PeerId:          peerReq.PeerId,
		Qnr:             qnr,
	}, nil
}

// ListPeers lists the peers connected to this node.
func (ns *Server) ListPeers(ctx context.Context, _ *emptypb.Empty) (*qrysmpb.Peers, error) {
	peers := ns.PeersFetcher.Peers().Connected()
	res := make([]*qrysmpb.Peer, 0, len(peers))
	for _, pid := range peers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		multiaddr, err := ns.PeersFetcher.Peers().Address(pid)
		if err != nil {
			continue
		}
		direction, err := ns.PeersFetcher.Peers().Direction(pid)
		if err != nil {
			continue
		}
		record, err := ns.PeersFetcher.Peers().QNR(pid)
		if err != nil {
			continue
		}
		qnr := ""
		if record != nil {
			qnr, err = p2p.SerializeQNR(record)
			if err != nil {
				continue
			}
		}
		multiAddrStr := "unknown"
		if multiaddr != nil {
			multiAddrStr = multiaddr.String()
		}
		address := fmt.Sprintf("%s/p2p/%s", multiAddrStr, pid.String())
		pbDirection := qrysmpb.PeerDirection_UNKNOWN
		switch direction {
		case network.DirInbound:
			pbDirection = qrysmpb.PeerDirection_INBOUND
		case network.DirOutbound:
			pbDirection = qrysmpb.PeerDirection_OUTBOUND
		}
		res = append(res, &qrysmpb.Peer{
			Address:         address,
			Direction:       pbDirection,
			ConnectionState: qrysmpb.ConnectionState_CONNECTED,
			PeerId:          pid.String(),
			Qnr:             qnr,
		})
	}

	return &qrysmpb.Peers{
		Peers: res,
	}, nil
}

// GetExecutionConnectionStatus gets data about the execution endpoints.
func (ns *Server) GetExecutionConnectionStatus(_ context.Context, _ *emptypb.Empty) (*qrysmpb.ExecutionConnectionStatus, error) {
	var currErr string
	err := ns.ExecutionChainInfoFetcher.ExecutionClientConnectionErr()
	if err != nil {
		currErr = err.Error()
	}
	return &qrysmpb.ExecutionConnectionStatus{
		CurrentAddress:         ns.ExecutionChainInfoFetcher.ExecutionClientEndpoint(),
		CurrentConnectionError: currErr,
		Addresses:              []string{ns.ExecutionChainInfoFetcher.ExecutionClientEndpoint()},
	}, nil
}
