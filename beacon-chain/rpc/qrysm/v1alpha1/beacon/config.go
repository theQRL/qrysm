package beacon

import (
	"context"
	"fmt"
	"reflect"

	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetBeaconConfig retrieves the current configuration parameters of the beacon chain.
func (_ *Server) GetBeaconConfig(_ context.Context, _ *emptypb.Empty) (*qrysmpb.BeaconConfig, error) {
	conf := params.BeaconConfig()
	val := reflect.ValueOf(conf).Elem()
	numFields := val.Type().NumField()
	res := make(map[string]string, numFields)
	for i := range numFields {
		res[val.Type().Field(i).Name] = fmt.Sprintf("%v", val.Field(i).Interface())
	}
	return &qrysmpb.BeaconConfig{
		Config: res,
	}, nil
}
