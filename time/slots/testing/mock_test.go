package testing

import (
	"github.com/theQRL/qrysm/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
