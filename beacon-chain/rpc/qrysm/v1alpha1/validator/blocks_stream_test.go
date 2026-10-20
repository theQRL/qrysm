package validator

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/qrysm/async/event"
	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	blockfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/block"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controlledBlockStream struct {
	qrysmpb.BeaconNodeValidator_StreamBlocksAltairServer
	ctx  context.Context
	send func(*qrysmpb.StreamBlocksResponse) error
}

func (s *controlledBlockStream) Context() context.Context { return s.ctx }
func (s *controlledBlockStream) Send(b *qrysmpb.StreamBlocksResponse) error {
	return s.send(b)
}

func blockStreamFixture(t *testing.T, verified bool) (*Server, *event.Feed, *feed.Event) {
	t.Helper()
	setupBlockStreamTest(t)
	st, keys := util.DeterministicGenesisStateZond(t, 64)
	pb, err := util.GenerateFullBlockZond(st.Copy(), keys, &util.BlockGenConfig{}, 1)
	require.NoError(t, err)
	b, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	chain := &chainMock.ChainService{State: st}
	srv := &Server{HeadFetcher: chain, StateNotifier: chain.StateNotifier(), BlockNotifier: chain.BlockNotifier()}
	if verified {
		return srv, srv.StateNotifier.StateFeed(), &feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{SignedBlock: b, Verified: true},
		}
	}
	return srv, srv.BlockNotifier.BlockFeed(), &feed.Event{
		Type: blockfeed.ReceivedBlock,
		Data: &blockfeed.ReceivedBlockData{SignedBlock: b},
	}
}

func TestStreamBlocksAltair_SlowReaderDoesNotBlockFeed(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(map[bool]string{false: "received blocks", true: "verified blocks"}[verified], func(t *testing.T) {
			srv, f, ev := blockStreamFixture(t, verified)
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				srv.Ctx = ctx
				entered, release := make(chan struct{}), make(chan struct{})
				defer close(release)
				stream := &controlledBlockStream{ctx: ctx, send: func(*qrysmpb.StreamBlocksResponse) error {
					close(entered)
					<-release
					return context.Canceled
				}}
				result := make(chan error, 1)
				go func() { result <- srv.StreamBlocksAltair(&qrysmpb.StreamBlocksRequest{VerifiedOnly: verified}, stream) }()
				synctest.Wait() // The stream is now subscribed and waiting for a block.
				const numEvents = blockStreamBufferSize + 2
				healthy := make(chan *feed.Event, numEvents)
				sub := f.Subscribe(healthy)
				defer sub.Unsubscribe()
				require.Equal(t, 2, f.Send(ev))
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("block was not delivered to the transport")
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					for range numEvents - 1 {
						f.Send(ev)
					}
				}()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("feed producer blocked behind the stalled block stream")
				}
				select {
				case err := <-result:
					require.Equal(t, codes.ResourceExhausted, status.Code(err), "unexpected error: %v", err)
				case <-time.After(time.Second):
					t.Fatal("slow stream did not return while its transport Send was blocked")
				}
				require.Equal(t, numEvents, len(healthy), "other subscribers must receive every event")
				for range numEvents {
					require.Equal(t, ev, <-healthy)
				}
				require.Equal(t, 1, f.Send(ev), "the slow stream must have unsubscribed")
			})
		})
	}
}

func TestStreamBlocksAltair_ExitDuringSend(t *testing.T) {
	for _, verified := range []bool{false, true} {
		for _, mode := range []string{"send error", "stream canceled", "service stopped"} {
			t.Run(map[bool]string{false: "received/", true: "verified/"}[verified]+mode, func(t *testing.T) {
				srv, f, ev := blockStreamFixture(t, verified)
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					srvCtx, srvCancel := context.WithCancel(context.Background())
					srv.Ctx = srvCtx
					defer srvCancel()
					entered, release := make(chan struct{}), make(chan struct{})
					defer close(release)
					stream := &controlledBlockStream{ctx: ctx, send: func(*qrysmpb.StreamBlocksResponse) error {
						close(entered)
						if mode != "send error" {
							<-release
						}
						return errors.New("transport send failed")
					}}
					result := make(chan error, 1)
					go func() { result <- srv.StreamBlocksAltair(&qrysmpb.StreamBlocksRequest{VerifiedOnly: verified}, stream) }()
					synctest.Wait()
					require.Equal(t, 1, f.Send(ev))
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("block was not delivered to the transport")
					}
					wantCode := codes.Canceled
					switch mode {
					case "send error":
						wantCode = codes.Unavailable
					case "stream canceled":
						cancel()
					case "service stopped":
						srvCancel()
					}
					select {
					case err := <-result:
						require.Equal(t, wantCode, status.Code(err), "unexpected error: %v", err)
					case <-time.After(time.Second):
						t.Fatal("stream did not stop")
					}
					require.Equal(t, 0, f.Send(ev), "terminated stream must have unsubscribed")
				})
			})
		}
	}
}

func TestStreamBlocksAltair_QueuedBlocksStayOrdered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		chain := &chainMock.ChainService{}
		srv := &Server{Ctx: ctx, StateNotifier: chain.StateNotifier()}
		f := srv.StateNotifier.StateFeed()
		entered, release := make(chan struct{}), make(chan struct{}, 1)
		defer close(release)
		received := make(chan *qrysmpb.StreamBlocksResponse, 3)
		first := true
		stream := &controlledBlockStream{ctx: ctx, send: func(b *qrysmpb.StreamBlocksResponse) error {
			if first {
				first = false
				close(entered)
				<-release
			}
			received <- b
			return nil
		}}
		result := make(chan error, 1)
		go func() { result <- srv.StreamBlocksAltair(&qrysmpb.StreamBlocksRequest{VerifiedOnly: true}, stream) }()
		synctest.Wait()
		var want []*qrysmpb.SignedBeaconBlockZond
		for i := range 3 {
			pb := util.NewBeaconBlockZond()
			pb.Block.Body.Graffiti[0] = byte(i)
			want = append(want, pb)
			b, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			f.Send(&feed.Event{Type: statefeed.BlockProcessed, Data: &statefeed.BlockProcessedData{SignedBlock: b, Verified: true}})
			if i == 0 {
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("first block was not delivered")
				}
			}
		}
		// Other state events must not fill the block outbox or disconnect an
		// otherwise healthy client while it finishes its first block.
		for range blockStreamBufferSize + 1 {
			f.Send(&feed.Event{Type: statefeed.MissedSlot})
		}
		release <- struct{}{}
		for _, pb := range want {
			select {
			case got := <-received:
				require.DeepEqual(t, pb, got.GetZondBlock())
			case <-time.After(time.Second):
				t.Fatal("queued block was not delivered")
			}
		}
		cancel()
		require.Equal(t, codes.Canceled, status.Code(<-result))
		require.Equal(t, 0, f.Send(&feed.Event{Type: statefeed.MissedSlot}))
	})
}
