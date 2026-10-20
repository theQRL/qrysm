package sync

import (
	"context"
	"reflect"
	"runtime/debug"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/monitoring/tracing"
	"github.com/theQRL/qrysm/time"
	"go.opencensus.io/trace"
)

// Time to first byte timeout. The maximum time to wait for first byte of
// request response (time-to-first-byte). The client is expected to give up if
// they don't receive the first byte within 5 seconds.
var ttfbTimeout = params.BeaconNetworkConfig().TtfbTimeout

// respTimeout is the maximum time for complete response transfer.
var respTimeout = params.BeaconNetworkConfig().RespTimeout

// rpcHandler is responsible for handling and responding to any incoming message.
// This method may return an error to internal monitoring, but the error will
// not be relayed to the peer.
type rpcHandler func(context.Context, any, libp2pcore.Stream) error

// registerRPCHandlers for p2p RPC.
func (s *Service) registerRPCHandlers() {
	s.registerRPC(
		p2p.RPCStatusTopicV1,
		s.statusRPCHandler,
	)
	s.registerRPC(
		p2p.RPCGoodByeTopicV1,
		s.goodbyeRPCHandler,
	)
	s.registerRPC(
		p2p.RPCPingTopicV1,
		s.pingHandler,
	)
	s.registerRPCHandlersAltair()
}

// registerRPCHandlers for altair.
func (s *Service) registerRPCHandlersAltair() {
	s.registerRPC(
		p2p.RPCBlocksByRangeTopicV2,
		s.beaconBlocksByRangeRPCHandler,
	)
	s.registerRPC(
		p2p.RPCBlocksByRootTopicV2,
		s.beaconBlocksRootRPCHandler,
	)
	s.registerRPC(
		p2p.RPCMetaDataTopicV2,
		s.metaDataHandler,
	)
}

// registerRPC for a given topic with an expected protobuf message type.
func (s *Service) registerRPC(baseTopic string, handle rpcHandler) {
	topic := baseTopic + s.cfg.p2p.Encoding().ProtocolSuffix()
	log := log.WithField("topic", topic)
	s.cfg.p2p.SetStreamHandler(topic, func(stream network.Stream) {
		defer func() {
			if r := recover(); r != nil {
				log.WithField("error", r).
					WithField("recovered_at", "registerRPC").
					WithField("stack", string(debug.Stack())).
					Error("Panic occurred")
			}
		}()

		ctx, cancel := context.WithTimeout(s.ctx, ttfbTimeout)
		defer cancel()

		// Resetting after closing is a no-op so defer a reset in case something goes wrong.
		// It's up to the handler to Close the stream (send an EOF) if
		// it successfully writes a response. We don't blindly call
		// Close here because we may have only written a partial
		// response.
		defer func() {
			_err := stream.Reset()
			_ = _err
		}()

		ctx, span := trace.StartSpan(ctx, "sync.rpc")
		defer span.End()
		span.AddAttributes(trace.StringAttribute("topic", topic))
		pid := stream.Conn().RemotePeer()
		span.AddAttributes(trace.StringAttribute("peer", pid.String()))
		log := log.WithField("peer", pid.String()).WithField("topic", string(stream.Protocol()))

		// Check before hand that peer is valid.
		if s.cfg.p2p.Peers().IsBad(pid) {
			if err := s.sendGoodByeAndDisconnect(ctx, p2ptypes.GoodbyeCodeBanned, pid); err != nil {
				log.WithError(err).Debug("Could not disconnect from peer")
			}
			return
		}
		// Validate request according to peer limits.
		if err := s.rateLimiter.validateRawRpcRequest(stream, 1); err != nil {
			log.WithError(err).Debug("Could not validate rpc request from peer")
			return
		}
		s.rateLimiter.addRawStream(stream)

		if err := stream.SetReadDeadline(time.Now().Add(ttfbTimeout)); err != nil {
			log.WithError(err).Debug("Could not set stream read deadline")
			return
		}

		base, ok := p2p.RPCTopicMappings[baseTopic]
		if !ok {
			log.Errorf("Could not retrieve base message for topic %s", baseTopic)
			return
		}
		t := reflect.TypeOf(base)
		// Copy Base
		base = reflect.New(t)

		// Increment message received counter.
		messageReceivedCounter.WithLabelValues(topic).Inc()

		// since metadata requests do not have any data in the payload, we
		// do not decode anything.
		if baseTopic == p2p.RPCMetaDataTopicV2 {
			if err := handle(ctx, base, stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if err != p2ptypes.ErrWrongForkDigestVersion {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
			return
		}

		// Given we have an input argument that can be pointer or the actual object, this gives us
		// a way to check for its reflect.Kind and based on the result, we can decode
		// accordingly.
		if t.Kind() == reflect.Ptr {
			msg, ok := reflect.New(t.Elem()).Interface().(ssz.Unmarshaler)
			if !ok {
				log.Errorf("message of %T does not support marshaller interface", msg)
				return
			}
			if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
				log.WithError(err).WithField("topic", topic).Debug("Could not decode stream message")
				tracing.AnnotateError(span, err)
				s.downscorePeer(pid, "registerRpcDecodeError")
				return
			}
			if err := handle(ctx, msg, stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if err != p2ptypes.ErrWrongForkDigestVersion {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
		} else {
			nTyp := reflect.New(t)
			msg, ok := nTyp.Interface().(ssz.Unmarshaler)
			if !ok {
				log.Errorf("message of %T does not support marshaller interface", msg)
				return
			}
			if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
				log.WithError(err).WithField("topic", topic).Debug("Could not decode stream message")
				tracing.AnnotateError(span, err)
				s.downscorePeer(pid, "registerRpcDecodeError")
				return
			}
			if err := handle(ctx, nTyp.Elem().Interface(), stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if err != p2ptypes.ErrWrongForkDigestVersion {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
		}
	})
}

// downscorePeer increments the bad-responses counter for the peer and emits a single
// debug log carrying the new score. Reasons should be stable identifiers so logs are greppable.
func (s *Service) downscorePeer(peerID peer.ID, reason string) {
	newScore := s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{
		"peerID":   peerID,
		"reason":   reason,
		"newScore": newScore,
	}).Debug("Downscore peer")
}
