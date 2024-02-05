package payloadattribute

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

var (
	_ = Attributer(&data{})
)

type data struct {
	version               int
	timeStamp             uint64
	prevRandao            []byte
	suggestedFeeRecipient []byte
	withdrawals           []*enginev1.Withdrawal
	// parentBeaconBlockRoot []byte TODO(rgeraldes24): field `parentBeaconBlockRoot` is unused (golangci)
}

var (
	errNilPayloadAttribute         = errors.New("received nil payload attribute")
	errUnsupportedPayloadAttribute = errors.New("unsupported payload attribute")
)

// New returns a new payload attribute with the given input object.
func New(i interface{}) (Attributer, error) {
	switch a := i.(type) {
	case nil:
		return nil, blocks.ErrNilObject
	case *enginev1.PayloadAttributesV2:
		return initPayloadAttributeFromV2(a)
	default:
		return nil, errors.Wrapf(errUnsupportedPayloadAttribute, "unable to create payload attribute from type %T", i)
	}
}

// EmptyWithVersion returns an empty payload attribute with the given version.
func EmptyWithVersion(version int) Attributer {
	return &data{
		version: version,
	}
}

func initPayloadAttributeFromV2(a *enginev1.PayloadAttributesV2) (Attributer, error) {
	if a == nil {
		return nil, errNilPayloadAttribute
	}

	return &data{
		version:               version.Capella,
		prevRandao:            a.PrevRandao,
		timeStamp:             a.Timestamp,
		suggestedFeeRecipient: a.SuggestedFeeRecipient,
		withdrawals:           a.Withdrawals,
	}, nil
}
