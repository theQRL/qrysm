package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	gomplex "github.com/libp2p/go-mplex"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/config/features"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	"github.com/theQRL/qrysm/runtime/version"
)

// setConnManagerOption returns a libp2p.ConnectionManager option whose high-water mark is the
// larger of the libp2p default and MaxPeers + 32. This prevents the manager from pruning peers
// that the operator opted in to via --p2p-max-peers — without it, libp2p's default high-water
// mark (192) prunes connections whenever MaxPeers > 192.
func setConnManagerOption(cfg *Config) libp2p.Option {
	low, high := cfg.connManagerLowHigh()
	cm, err := connmgr.NewConnManager(low, high)
	if err != nil {
		log.WithError(err).Fatal("Failed to create libp2p connection manager")
	}
	return libp2p.ConnectionManager(cm)
}

// MultiAddressBuilder takes in an ip address string and port to produce a go multiaddr format.
func MultiAddressBuilder(ipAddr string, port uint) (ma.Multiaddr, error) {
	parsedIP := net.ParseIP(ipAddr)
	if parsedIP.To4() == nil && parsedIP.To16() == nil {
		return nil, errors.Errorf("invalid ip address provided: %s", ipAddr)
	}
	if parsedIP.To4() != nil {
		return ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipAddr, port))
	}
	return ma.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/%d", ipAddr, port))
}

// buildOptions for the libp2p host.
func (s *Service) buildOptions(ip net.IP, priKey *ecdsa.PrivateKey) []libp2p.Option {
	cfg := s.cfg
	listen, err := MultiAddressBuilder(ip.String(), cfg.TCPPort)
	if err != nil {
		log.WithError(err).Fatal("Failed to p2p listen")
	}
	if cfg.LocalIP != "" {
		if net.ParseIP(cfg.LocalIP) == nil {
			log.Fatalf("Invalid local ip provided: %s", cfg.LocalIP)
		}
		listen, err = MultiAddressBuilder(cfg.LocalIP, cfg.TCPPort)
		if err != nil {
			log.WithError(err).Fatal("Failed to p2p listen")
		}
	}
	ifaceKey, err := ecdsaqrysm.ConvertToInterfacePrivkey(priKey)
	if err != nil {
		log.WithError(err).Fatal("Failed to retrieve private key")
	}
	id, err := peer.IDFromPublicKey(ifaceKey.GetPublic())
	if err != nil {
		log.WithError(err).Fatal("Failed to retrieve peer id")
	}
	log.Infof("Running node with peer id of %s ", id.String())

	options := []libp2p.Option{
		privKeyOption(priKey),
		libp2p.ListenAddrs(listen),
		libp2p.UserAgent(version.BuildData()),
		libp2p.ConnectionGater(s),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.DefaultMuxers,
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
	}

	options = append(options, libp2p.Security(noise.ID, noise.New))
	options = append(options, setConnManagerOption(cfg))

	if cfg.EnableUPnP {
		options = append(options, libp2p.NATPortMap()) // Allow to use UPnP
	}
	if cfg.RelayNodeAddr != "" {
		options = append(options, libp2p.AddrsFactory(withRelayAddrs(cfg.RelayNodeAddr)))
	} else {
		// Disable relay if it has not been set.
		options = append(options, libp2p.DisableRelay())
	}
	if cfg.HostAddress != "" {
		options = append(options, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			external, err := MultiAddressBuilder(cfg.HostAddress, cfg.TCPPort)
			if err != nil {
				log.WithError(err).Error("Unable to create external multiaddress")
			} else {
				addrs = append(addrs, external)
			}
			return addrs
		}))
	}
	if cfg.HostDNS != "" {
		options = append(options, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			external, err := ma.NewMultiaddr(fmt.Sprintf("/dns4/%s/tcp/%d", cfg.HostDNS, cfg.TCPPort))
			if err != nil {
				log.WithError(err).Error("Unable to create external multiaddress")
			} else {
				addrs = append(addrs, external)
			}
			return addrs
		}))
	}
	// Disable Ping Service.
	options = append(options, libp2p.Ping(false))
	if features.Get().DisableResourceManager {
		options = append(options, libp2p.ResourceManager(&network.NullResourceManager{}))
	}
	return options
}

func multiAddressBuilderWithID(ipAddr, protocol string, port uint, id peer.ID) (ma.Multiaddr, error) {
	parsedIP := net.ParseIP(ipAddr)
	if parsedIP.To4() == nil && parsedIP.To16() == nil {
		return nil, errors.Errorf("invalid ip address provided: %s", ipAddr)
	}
	if id.String() == "" {
		return nil, errors.New("empty peer id given")
	}
	if parsedIP.To4() != nil {
		return ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/%s/%d/p2p/%s", ipAddr, protocol, port, id.String()))
	}
	return ma.NewMultiaddr(fmt.Sprintf("/ip6/%s/%s/%d/p2p/%s", ipAddr, protocol, port, id.String()))
}

// Adds a private key to the libp2p option if the option was provided.
// If the private key file is missing or cannot be read, or if the
// private key contents cannot be marshaled, an exception is thrown.
func privKeyOption(privkey *ecdsa.PrivateKey) libp2p.Option {
	return func(cfg *libp2p.Config) error {
		ifaceKey, err := ecdsaqrysm.ConvertToInterfacePrivkey(privkey)
		if err != nil {
			return err
		}
		log.Debug("ECDSA private key generated")
		return cfg.Apply(libp2p.Identity(ifaceKey))
	}
}

// configureMplex caps how long a half-closed mplex stream can hang around,
// limiting resource use from misbehaving or slow peers.
func configureMplex() {
	gomplex.ResetStreamTimeout = 5 * time.Second
}
