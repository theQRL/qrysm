package migration

import (
	"testing"

	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	v1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
)

func TestV1Alpha1ConnectionStateToV1(t *testing.T) {
	tests := []struct {
		name      string
		connState zond.ConnectionState
		want      v1.ConnectionState
	}{
		{
			name:      "DISCONNECTED",
			connState: zond.ConnectionState_DISCONNECTED,
			want:      v1.ConnectionState_DISCONNECTED,
		},
		{
			name:      "CONNECTED",
			connState: zond.ConnectionState_CONNECTED,
			want:      v1.ConnectionState_CONNECTED,
		},
		{
			name:      "CONNECTING",
			connState: zond.ConnectionState_CONNECTING,
			want:      v1.ConnectionState_CONNECTING,
		},
		{
			name:      "DISCONNECTING",
			connState: zond.ConnectionState_DISCONNECTING,
			want:      v1.ConnectionState_DISCONNECTING,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := V1Alpha1ConnectionStateToV1(tt.connState); got != tt.want {
				t.Errorf("V1Alpha1ConnectionStateToV1() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestV1Alpha1PeerDirectionToV1(t *testing.T) {
	tests := []struct {
		name          string
		peerDirection zond.PeerDirection
		want          v1.PeerDirection
		wantErr       bool
	}{
		{
			name:          "UNKNOWN",
			peerDirection: zond.PeerDirection_UNKNOWN,
			want:          0,
			wantErr:       true,
		},
		{
			name:          "INBOUND",
			peerDirection: zond.PeerDirection_INBOUND,
			want:          v1.PeerDirection_INBOUND,
		},
		{
			name:          "OUTBOUND",
			peerDirection: zond.PeerDirection_OUTBOUND,
			want:          v1.PeerDirection_OUTBOUND,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := V1Alpha1PeerDirectionToV1(tt.peerDirection)
			if (err != nil) != tt.wantErr {
				t.Errorf("V1Alpha1PeerDirectionToV1() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("V1Alpha1PeerDirectionToV1() got = %v, want %v", got, tt.want)
			}
		})
	}
}
