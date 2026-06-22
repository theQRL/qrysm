package migration

import (
	"github.com/pkg/errors"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func V1Alpha1ConnectionStateToV1(connState qrysmpb.ConnectionState) qrlpb.ConnectionState {
	alphaString := connState.String()
	v1Value := qrlpb.ConnectionState_value[alphaString]
	return qrlpb.ConnectionState(v1Value)
}

func V1Alpha1PeerDirectionToV1(peerDirection qrysmpb.PeerDirection) (qrlpb.PeerDirection, error) {
	alphaString := peerDirection.String()
	if alphaString == qrysmpb.PeerDirection_UNKNOWN.String() {
		return 0, errors.New("peer direction unknown")
	}
	v1Value := qrlpb.PeerDirection_value[alphaString]
	return qrlpb.PeerDirection(v1Value), nil
}
