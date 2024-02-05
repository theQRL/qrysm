package payloadattribute

import (
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
)

type Attributer interface {
	Version() int
	PrevRandao() []byte
	Timestamps() uint64
	SuggestedFeeRecipient() []byte
	Withdrawals() ([]*enginev1.Withdrawal, error)
	PbV2() (*enginev1.PayloadAttributesV2, error)
}
