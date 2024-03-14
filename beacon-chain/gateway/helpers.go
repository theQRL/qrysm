package gateway

import (
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/theQRL/qrysm/v4/api/gateway"
	"github.com/theQRL/qrysm/v4/cmd/beacon-chain/flags"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbservice "github.com/theQRL/qrysm/v4/proto/zond/service"
	"google.golang.org/protobuf/encoding/protojson"
)

// MuxConfig contains configuration that should be used when registering the beacon node in the gateway.
type MuxConfig struct {
	Handler      gateway.MuxHandler
	ZondPbMux    *gateway.PbMux
	V1AlphaPbMux *gateway.PbMux
}

// DefaultConfig returns a fully configured MuxConfig with standard gateway behavior.
func DefaultConfig(enableDebugRPCEndpoints bool, httpModules string) MuxConfig {
	var v1AlphaPbHandler, zondPbHandler *gateway.PbMux
	if flags.EnableHTTPQrysmAPI(httpModules) {
		v1AlphaRegistrations := []gateway.PbHandlerRegistration{
			zondpbalpha.RegisterNodeHandler,
			zondpbalpha.RegisterBeaconChainHandler,
			zondpbalpha.RegisterBeaconNodeValidatorHandler,
		}
		if enableDebugRPCEndpoints {
			v1AlphaRegistrations = append(v1AlphaRegistrations, zondpbalpha.RegisterDebugHandler)
		}
		v1AlphaMux := gwruntime.NewServeMux(
			gwruntime.WithMarshalerOption(gwruntime.MIMEWildcard, &gwruntime.HTTPBodyMarshaler{
				Marshaler: &gwruntime.JSONPb{
					MarshalOptions: protojson.MarshalOptions{
						EmitUnpopulated: true,
					},
					UnmarshalOptions: protojson.UnmarshalOptions{
						DiscardUnknown: true,
					},
				},
			}),
			gwruntime.WithMarshalerOption(
				"text/event-stream", &gwruntime.EventSourceJSONPb{},
			),
		)
		v1AlphaPbHandler = &gateway.PbMux{
			Registrations: v1AlphaRegistrations,
			Patterns:      []string{"/zond/v1alpha1/", "/zond/v1alpha2/"},
			Mux:           v1AlphaMux,
		}
	}
	if flags.EnableHTTPZondAPI(httpModules) {
		ethRegistrations := []gateway.PbHandlerRegistration{
			zondpbservice.RegisterBeaconNodeHandler,
			zondpbservice.RegisterBeaconChainHandler,
			zondpbservice.RegisterBeaconValidatorHandler,
			zondpbservice.RegisterEventsHandler,
		}
		if enableDebugRPCEndpoints {
			ethRegistrations = append(ethRegistrations, zondpbservice.RegisterBeaconDebugHandler)
		}
		ethMux := gwruntime.NewServeMux(
			gwruntime.WithMarshalerOption(gwruntime.MIMEWildcard, &gwruntime.HTTPBodyMarshaler{
				Marshaler: &gwruntime.JSONPb{
					MarshalOptions: protojson.MarshalOptions{
						UseProtoNames:   true,
						EmitUnpopulated: true,
					},
					UnmarshalOptions: protojson.UnmarshalOptions{
						DiscardUnknown: true,
					},
				},
			}),
		)
		zondPbHandler = &gateway.PbMux{
			Registrations: ethRegistrations,
			Patterns:      []string{"/internal/zond/v1/"},
			Mux:           ethMux,
		}
	}

	return MuxConfig{
		ZondPbMux:    zondPbHandler,
		V1AlphaPbMux: v1AlphaPbHandler,
	}
}
