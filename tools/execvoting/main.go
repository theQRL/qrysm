package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	v1alpha1 "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	beacon  = flag.String("beacon", "127.0.0.1:4000", "gRPC address of the Qrysm beacon node")
	genesis = flag.Uint64("genesis", 1606824023, "Genesis time. mainnet=1606824023, prater=1616508000")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	cc, err := grpc.DialContext(ctx, *beacon, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt64)))
	if err != nil {
		panic(err)
	}
	c := v1alpha1.NewBeaconChainClient(cc)
	g, ctx := errgroup.WithContext(ctx)
	v := newVotes()

	current := slots.ToEpoch(slots.CurrentSlot(*genesis))
	start := current.Div(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod)).Mul(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod))
	nextStart := start.AddEpoch(params.BeaconConfig().EpochsPerExecutionVotingPeriod)

	fmt.Printf("Looking back from current epoch %d back to %d\n", current, start)
	nextStartSlot, err := slots.EpochStart(nextStart)
	if err != nil {
		panic(err)
	}
	nextStartTime, err := slots.ToTime(*genesis, nextStartSlot)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Next period starts at epoch %d (%s)\n", nextStart, time.Until(nextStartTime))

	for i := primitives.Epoch(0); i < current.Sub(uint64(start)); i++ {
		j := i
		g.Go(func() error {
			resp, err := c.ListBeaconBlocks(ctx, &v1alpha1.ListBlocksRequest{
				QueryFilter: &v1alpha1.ListBlocksRequest_Epoch{Epoch: current.Sub(uint64(j))},
			})
			if err != nil {
				return err
			}
			for _, c := range resp.GetBlockContainers() {
				v.Insert(wrapBlock(c))
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		panic(err)
	}

	fmt.Println(v.Report())
}

func wrapBlock(b *v1alpha1.BeaconBlockContainer) interfaces.ReadOnlyBeaconBlock {
	var err error
	var wb interfaces.ReadOnlySignedBeaconBlock
	switch bb := b.Block.(type) {
	case *v1alpha1.BeaconBlockContainer_ZondBlock:
		wb, err = blocks.NewSignedBeaconBlock(bb.ZondBlock)
	case *v1alpha1.BeaconBlockContainer_BlindedZondBlock:
		wb, err = blocks.NewSignedBeaconBlock(bb.BlindedZondBlock)
	}
	if err != nil {
		panic("no block")
	}
	return wb.Block()
}
