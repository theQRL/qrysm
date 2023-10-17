package testing

import (
	"github.com/theQRL/qrysm/v4/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
