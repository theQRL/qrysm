package kv

import (
	"context"
	"testing"

	testpb "github.com/theQRL/qrysm/proto/testing"
	"github.com/theQRL/qrysm/testing/require"
)

func Test_encode_handlesNilFromFunction(t *testing.T) {
	foo := func() *testpb.Puzzle {
		return nil
	}
	_, err := encode(context.Background(), foo())
	require.ErrorContains(t, "cannot encode nil message", err)
}
