package p2p

import (
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/time/slots"
)

// A background routine which listens for new and upcoming forks and
// updates the node's discovery service to reflect any new fork version
// changes.
func (s *Service) forkWatcher() {
	slotTicker := slots.NewSlotTicker(s.genesisTime, params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		case currSlot := <-slotTicker.C():
			currEpoch := slots.ToEpoch(currSlot)
			if currEpoch == 0 {
				// If we are in the fork epoch, we update our enr with
				// the updated fork digest. These repeatedly does
				// this over the epoch, which might be slightly wasteful
				// but is fine nonetheless.
				if s.dv5Listener != nil { // make sure it's not a local network
					_, err := addForkEntry(s.dv5Listener.LocalNode(), s.genesisTime, s.genesisValidatorsRoot)
					if err != nil {
						log.WithError(err).Error("Could not add fork entry")
					}
				}
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			slotTicker.Done()
			return
		}
	}
}
