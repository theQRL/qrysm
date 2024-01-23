package components

import (
	"context"
	"fmt"
	mathRand "math/rand"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-zond/rpc"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/rand"
	e2e "github.com/theQRL/qrysm/v4/testing/endtoend/params"
)

type TransactionGenerator struct {
	keystore string
	seed     int64
	started  chan struct{}
	cancel   context.CancelFunc
}

func NewTransactionGenerator(keystore string, seed int64) *TransactionGenerator {
	return &TransactionGenerator{keystore: keystore, seed: seed}
}

func (t *TransactionGenerator) Start(ctx context.Context) error {
	// Wrap context with a cancel func
	ctx, ccl := context.WithCancel(ctx)
	t.cancel = ccl

	client, err := rpc.DialHTTP(fmt.Sprintf("http://127.0.0.1:%d", e2e.TestParams.Ports.ZondRPCPort))
	if err != nil {
		return err
	}
	defer client.Close()

	seed := t.seed
	if seed == 0 {
		seed = rand.NewDeterministicGenerator().Int63()
		logrus.Infof("Seed for transaction generator is: %d", seed)
	}
	// Set seed so that all transactions can be
	// deterministically generated.
	mathRand.Seed(seed)

	// keystoreBytes, err := os.ReadFile(t.keystore) // #nosec G304
	// if err != nil {
	// 	return err
	// }
	// mineKey, err := keystore.DecryptKey(keystoreBytes, KeystorePassword)
	// if err != nil {
	// 	return err
	// }
	rnd := make([]byte, 10000)
	// #nosec G404
	_, err = mathRand.Read(rnd)
	if err != nil {
		return err
	}
	// f := filler.NewFiller(rnd)
	// Broadcast Transactions every 3 blocks
	txPeriod := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	ticker := time.NewTicker(txPeriod)
	// gasPrice := big.NewInt(1e11)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			/*
				err := SendTransaction(client, mineKey.Dilithium, f, gasPrice, mineKey.Address.String(), 100, false)
				if err != nil {
					return err
				}
			*/
		}
	}
}

// Started checks whether beacon node set is started and all nodes are ready to be queried.
func (s *TransactionGenerator) Started() <-chan struct{} {
	return s.started
}

// TODO(rgeraldes24) - fix tx-fuzz dep
/*
func SendTransaction(client *rpc.Client, key *dilithium.Dilithium, f *filler.Filler, gasPrice *big.Int, addr string, N uint64, al bool) error {
	backend := zondclient.NewClient(client)

	sender := common.HexToAddress(addr)
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return err
	}
	nonce, err := backend.PendingNonceAt(context.Background(), sender)
	if err != nil {
		return err
	}
	expectedPrice, err := backend.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}
	if expectedPrice.Cmp(gasPrice) > 0 {
		gasPrice = expectedPrice
	}
	g, _ := errgroup.WithContext(context.Background())
	for i := uint64(0); i < N; i++ {
		index := i
		g.Go(func() error {
			tx, err := txfuzz.RandomValidTx(client, f, sender, nonce+index, gasPrice, nil, al)
			if err != nil {
				// In the event the transaction constructed is not valid, we continue with the routine
				// rather than complete stop it.
				//nolint:nilerr
				return nil
			}
			signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), key)
			if err != nil {
				// We continue on in the event there is a reason we can't sign this
				// transaction(unlikely).
				//nolint:nilerr
				return nil
			}
			err = backend.SendTransaction(context.Background(), signedTx)
			if err != nil {
				// We continue on if the constructed transaction is invalid
				// and can't be submitted on chain.
				//nolint:nilerr
				return nil
			}
			return nil
		})
	}
	return g.Wait()
}
*/

// Pause pauses the component and its underlying process.
func (t *TransactionGenerator) Pause() error {
	return nil
}

// Resume resumes the component and its underlying process.
func (t *TransactionGenerator) Resume() error {
	return nil
}

// Stop stops the component and its underlying process.
func (t *TransactionGenerator) Stop() error {
	t.cancel()
	return nil
}
