package beacon

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServer_GetBeaconConfig(t *testing.T) {
	ctx := context.Background()
	bs := &Server{}
	res, err := bs.GetBeaconConfig(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	conf := params.BeaconConfig()
	numFields := reflect.TypeFor[params.BeaconChainConfig]().NumField()

	// Check if the result has the same number of items as our config struct.
	assert.Equal(t, numFields, len(res.Config), "Unexpected number of items in config")
	want := fmt.Sprintf("%d", conf.ExecutionFollowDistance)

	// Check that an element is properly populated from the config.
	assert.Equal(t, want, res.Config["ExecutionFollowDistance"], "Unexpected follow distance")
}
