package gateway

import (
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/theQRL/qrysm/api/gateway"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/encoding/protojson"
)

// MuxConfig contains configuration that should be used when registering the beacon node in the gateway.
type MuxConfig struct {
	QRLPbMux   *gateway.PbMux
	QrysmPbMux *gateway.PbMux
}

// DefaultConfig returns a fully configured MuxConfig with standard gateway behavior.
func DefaultConfig(enableDebugRPCEndpoints bool, httpModules string) MuxConfig {
	var qrysmPbHandler, qrlPbHandler *gateway.PbMux
	if flags.EnableHTTPQrysmAPI(httpModules) {
		qrysmRegistrations := []gateway.PbHandlerRegistration{
			qrysmpb.RegisterNodeHandler,
			qrysmpb.RegisterBeaconChainHandler,
			qrysmpb.RegisterBeaconNodeValidatorHandler,
		}
		if enableDebugRPCEndpoints {
			qrysmRegistrations = append(qrysmRegistrations, qrysmpb.RegisterDebugHandler)
		}
		qrysmMux := gwruntime.NewServeMux(
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
		qrysmPbHandler = &gateway.PbMux{
			Registrations: qrysmRegistrations,
			Patterns:      []string{"/qrl/v1alpha1/", "/qrl/v1alpha2/"},
			Mux:           qrysmMux,
		}
	}
	if flags.EnableHTTPQRLAPI(httpModules) {
		qrlRegistrations := []gateway.PbHandlerRegistration{
			qrlpbservice.RegisterBeaconNodeHandler,
			qrlpbservice.RegisterBeaconChainHandler,
			qrlpbservice.RegisterBeaconValidatorHandler,
			qrlpbservice.RegisterEventsHandler,
		}
		if enableDebugRPCEndpoints {
			qrlRegistrations = append(qrlRegistrations, qrlpbservice.RegisterBeaconDebugHandler)
		}
		qrlMux := gwruntime.NewServeMux(
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
		qrlPbHandler = &gateway.PbMux{
			Registrations: qrlRegistrations,
			Patterns:      []string{"/internal/qrl/v1/"},
			Mux:           qrlMux,
		}
	}

	return MuxConfig{
		QRLPbMux:   qrlPbHandler,
		QrysmPbMux: qrysmPbHandler,
	}
}
