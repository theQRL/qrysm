package block

import "github.com/theQRL/qrysm/async/event"

// Notifier interface defines the methods of the service that provides block updates to consumers.
type Notifier interface {
	BlockFeed() *event.Feed
}
