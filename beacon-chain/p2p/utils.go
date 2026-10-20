package p2p

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/consensus-types/wrapper"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	"github.com/theQRL/qrysm/io/file"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/metadata"
)

const keyPath = "network-keys"

const dialTimeout = 1 * time.Second

// SerializeQNR takes the qnr record in its key-value form and serializes it.
func SerializeQNR(record *qnr.Record) (string, error) {
	if record == nil {
		return "", errors.New("could not serialize nil record")
	}
	buf := bytes.NewBuffer([]byte{})
	if err := record.EncodeRLP(buf); err != nil {
		return "", errors.Wrap(err, "could not encode QNR record to bytes")
	}
	qnrString := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	return qnrString, nil
}

// Determines a private key for p2p networking from the p2p service's
// configuration struct. If no key is found, it generates a new one.
func privKey(cfg *Config) (*ecdsa.PrivateKey, error) {
	defaultKeyPath := path.Join(cfg.DataDir, keyPath)
	privateKeyPath := cfg.PrivateKey

	// PrivateKey cli flag takes highest precedence.
	if privateKeyPath != "" {
		return privKeyFromFile(cfg.PrivateKey)
	}

	_, err := os.Stat(defaultKeyPath)
	defaultKeysExist := !os.IsNotExist(err)
	if err != nil && defaultKeysExist {
		return nil, err
	}
	// Default keys have the next highest precedence, if they exist.
	if defaultKeysExist {
		return privKeyFromFile(defaultKeyPath)
	}
	// There are no keys on the filesystem, so we need to generate one.
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	// If the StaticPeerID flag is set, save the generated key as the default
	// key, so that it will be used by default on the next node start.
	if cfg.StaticPeerID {
		rawbytes, err := priv.Raw()
		if err != nil {
			return nil, err
		}
		dst := make([]byte, hex.EncodedLen(len(rawbytes)))
		hex.Encode(dst, rawbytes)
		if err := file.WriteFile(defaultKeyPath, dst); err != nil {
			return nil, err
		}
		log.Infof("Wrote network key to file")
		// Read the key from the defaultKeyPath file just written
		// for the strongest guarantee that the next start will be the same as this one.
		return privKeyFromFile(defaultKeyPath)
	}
	return ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
}

// Retrieves a p2p networking private key from a file path.
func privKeyFromFile(path string) (*ecdsa.PrivateKey, error) {
	src, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		log.WithError(err).Error("Error reading private key from file")
		return nil, err
	}
	dst := make([]byte, hex.DecodedLen(len(src)))
	_, err = hex.Decode(dst, src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hex string")
	}
	unmarshalledKey, err := crypto.UnmarshalSecp256k1PrivateKey(dst)
	if err != nil {
		return nil, err
	}
	return ecdsaqrysm.ConvertFromInterfacePrivKey(unmarshalledKey)
}

// metaDataFromDB retrieves the p2p metadata sequence number from the database and returns
// a Metadata(V1) object. RefreshPersistentSubnets repopulates the subnet bitfields shortly
// after startup, so they're initialized empty here.
func metaDataFromDB(ctx context.Context, d db.ReadOnlyDatabaseWithSeqNum) (metadata.Metadata, error) {
	seqNum := uint64(0)
	if d != nil {
		s, err := d.MetadataSeqNum(ctx)
		// Proceed if the key isn't found by using the default sequence number of 0.
		if err != nil && !errors.Is(err, kv.ErrNotFoundMetadataSeqNum) {
			return nil, err
		}
		seqNum = s
	}

	return wrapper.WrappedMetadataV1(&pb.MetaDataV1{
		SeqNumber: seqNum,
		Attnets:   bitfield.NewBitvector64(),
		Syncnets:  bitfield.NewBitvector4(),
	}), nil
}

// Attempt to dial an address to verify its connectivity
func verifyConnectivity(addr string, port uint, protocol string) {
	if addr != "" {
		a := net.JoinHostPort(addr, fmt.Sprintf("%d", port))
		fields := logrus.Fields{
			"protocol": protocol,
			"address":  a,
		}
		conn, err := net.DialTimeout(protocol, a, dialTimeout)
		if err != nil {
			log.WithError(err).WithFields(fields).Warn("IP address is not accessible")
			return
		}
		if err := conn.Close(); err != nil {
			log.WithError(err).Debug("Could not close connection")
		}
	}
}
