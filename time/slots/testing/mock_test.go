package testing

import (
	"github.com/cyyber/qrysm/v4/time/slots"
)

var _ slots.Ticker = (*MockTicker)(nil)
