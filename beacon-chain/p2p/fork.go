package p2p

import (
	"bytes"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/network/forks"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

// QNR key used for QRL consensus-related fork data.
var consensusQNRKey = params.BeaconNetworkConfig().ConsensusKey

// ForkDigest returns the current fork digest of
// the node according to the local clock.
func (s *Service) currentForkDigest() ([4]byte, error) {
	if !s.isInitialized() {
		return [4]byte{}, errors.New("state is not initialized")
	}
	return forks.CreateForkDigest(s.genesisTime, s.genesisValidatorsRoot)
}

// Compares fork QNRs between an incoming peer's record and our node's
// local record values for current and next fork version/epoch.
func (s *Service) compareForkQNR(record *qnr.Record) error {
	currentRecord := s.dv5Listener.LocalNode().Node().Record()
	peerForkQNR, err := forkEntry(record)
	if err != nil {
		return err
	}
	currentForkQNR, err := forkEntry(currentRecord)
	if err != nil {
		return err
	}
	qnrString, err := SerializeQNR(record)
	if err != nil {
		return err
	}
	// Clients SHOULD connect to peers with current_fork_digest, next_fork_version,
	// and next_fork_epoch that match local values.
	if !bytes.Equal(peerForkQNR.CurrentForkDigest, currentForkQNR.CurrentForkDigest) {
		return fmt.Errorf(
			"fork digest of peer with QNR %s: %v, does not match local value: %v",
			qnrString,
			peerForkQNR.CurrentForkDigest,
			currentForkQNR.CurrentForkDigest,
		)
	}
	// Clients MAY connect to peers with the same current_fork_version but a
	// different next_fork_version/next_fork_epoch. Unless QNRForkID is manually
	// updated to matching prior to the earlier next_fork_epoch of the two clients,
	// these type of connecting clients will be unable to successfully interact
	// starting at the earlier next_fork_epoch.
	if peerForkQNR.NextForkEpoch != currentForkQNR.NextForkEpoch {
		log.WithFields(logrus.Fields{
			"peerNextForkEpoch":    peerForkQNR.NextForkEpoch,
			"currentNextForkEpoch": currentForkQNR.NextForkEpoch,
			"peerQNR":              qnrString,
		}).Trace("Peer matches fork digest but has different next fork epoch")
	}
	if !bytes.Equal(peerForkQNR.NextForkVersion, currentForkQNR.NextForkVersion) {
		log.WithFields(logrus.Fields{
			"peerNextForkVersion":    peerForkQNR.NextForkVersion,
			"currentNextForkVersion": currentForkQNR.NextForkVersion,
			"peerQNR":                qnrString,
		}).Trace("Peer matches fork digest but has different next fork version")
	}
	return nil
}

// Adds a fork entry as an QNR record under the QRL consensus QnrKey for
// the local node. The fork entry is an ssz-encoded qnrForkID type
// which takes into account the current fork version from the current
// epoch to create a fork digest, the next fork version,
// and the next fork epoch.
func addForkEntry(
	node *qnode.LocalNode,
	genesisTime time.Time,
	genesisValidatorsRoot []byte,
) (*qnode.LocalNode, error) {
	digest, err := forks.CreateForkDigest(genesisTime, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	currentSlot := slots.Since(genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)
	if qrysmTime.Now().Before(genesisTime) {
		currentEpoch = 0
	}
	nextForkVersion, nextForkEpoch, err := forks.NextForkData(currentEpoch)
	if err != nil {
		return nil, err
	}
	qnrForkID := &pb.QNRForkID{
		CurrentForkDigest: digest[:],
		NextForkVersion:   nextForkVersion[:],
		NextForkEpoch:     nextForkEpoch,
	}
	enc, err := qnrForkID.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	forkEntry := qnr.WithEntry(consensusQNRKey, enc)
	node.Set(forkEntry)
	return node, nil
}

// Retrieves an qnrForkID from an QNR record by key lookup
// under the QRL consensus QnrKey
func forkEntry(record *qnr.Record) (*pb.QNRForkID, error) {
	sszEncodedForkEntry := make([]byte, 16)
	entry := qnr.WithEntry(consensusQNRKey, &sszEncodedForkEntry)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	forkEntry := &pb.QNRForkID{}
	if err := forkEntry.UnmarshalSSZ(sszEncodedForkEntry); err != nil {
		return nil, err
	}
	return forkEntry, nil
}
