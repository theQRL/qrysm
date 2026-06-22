package p2p

import (
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/startup"
)

const (
	// defaultConnManagerPruneAbove is the libp2p ConnManager "high water mark" — the peer count
	// above which the manager begins pruning connections. Mirrors the libp2p default so we never
	// shrink the limit below it. The "low water mark" is the count where pruning stops, computed
	// by subtracting connManagerPruneAmount from the high water mark.
	defaultConnManagerPruneAbove = 192
	connManagerPruneAmount       = 32
)

// Config for the p2p service. These parameters are set from application level flags
// to initialize the p2p service.
type Config struct {
	NoDiscovery         bool
	EnableUPnP          bool
	StaticPeerID        bool
	StaticPeers         []string
	BootstrapNodeAddr   []string
	Discv5BootStrapAddr []string
	RelayNodeAddr       string
	LocalIP             string
	HostAddress         string
	HostDNS             string
	PrivateKey          string
	DataDir             string
	TCPPort             uint
	UDPPort             uint
	MaxPeers            uint
	AllowListCIDR       string
	DenyListCIDR        []string
	StateNotifier       statefeed.Notifier
	DB                  db.ReadOnlyDatabaseWithSeqNum
	ClockWaiter         startup.ClockWaiter
}

// connManagerLowHigh picks low/high water marks for the libp2p connection manager based on
// MaxPeers. The high water mark is at least the libp2p default (192) or MaxPeers + 32,
// whichever is higher; the low water mark is 32 below the high. This guarantees the
// ConnManager never prunes peers the node legitimately wants under the MaxPeers budget.
func (cfg *Config) connManagerLowHigh() (int, int) {
	maxPeersPlusMargin := int(cfg.MaxPeers) + connManagerPruneAmount
	high := max(maxPeersPlusMargin, defaultConnManagerPruneAbove)
	low := high - connManagerPruneAmount
	return low, high
}
