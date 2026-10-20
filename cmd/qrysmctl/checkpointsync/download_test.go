package checkpointsync

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theQRL/qrysm/api/client"
	"github.com/theQRL/qrysm/api/client/beacon"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestCLIActionDownload_LargeState(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	stateBytes, blockBytes := largeCheckpointFixture(t)
	require.Equal(t, true, int64(len(stateBytes)) > client.MaxBodySize)
	require.Equal(t, true, int64(len(stateBytes)) < beacon.MaxStateBodySize)
	t.Logf("checkpoint state size: %d bytes", len(stateBytes))

	for _, contentLength := range []int64{int64(len(stateBytes)), -1} {
		t.Run(fmt.Sprintf("content_length_%d", contentLength), func(t *testing.T) {
			t.Chdir(t.TempDir())
			stateBody := &trackedCheckpointBody{Reader: bytes.NewReader(stateBytes)}
			blockBody := &trackedCheckpointBody{Reader: bytes.NewReader(blockBytes)}
			var requests []string
			setDownloadTransport(t, func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "application/octet-stream", req.Header.Get("Accept"))
				requests = append(requests, req.URL.Path)
				res := &http.Response{StatusCode: http.StatusOK, Request: req}
				switch req.URL.Path {
				case "/qrl/v1/debug/beacon/states/finalized":
					res.Body, res.ContentLength = stateBody, contentLength
				case "/qrl/v1/beacon/blocks/0":
					res.Body, res.ContentLength = blockBody, int64(len(blockBytes))
				default:
					return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
				}
				return res, nil
			})

			require.NoError(t, cliActionDownload(nil))
			require.DeepEqual(t, []string{
				"/qrl/v1/debug/beacon/states/finalized",
				"/qrl/v1/beacon/blocks/0",
			}, requests)
			require.Equal(t, int64(len(stateBytes)), stateBody.read)
			require.Equal(t, int64(len(blockBytes)), blockBody.read)
			require.Equal(t, true, stateBody.closed)
			require.Equal(t, true, blockBody.closed)
			for prefix, want := range map[string][]byte{"state": stateBytes, "block": blockBytes} {
				paths, err := filepath.Glob(prefix + "_mainnet_zond_0-*.ssz")
				require.NoError(t, err)
				require.Equal(t, 1, len(paths))
				got, err := os.ReadFile(paths[0])
				require.NoError(t, err)
				require.Equal(t, true, bytes.Equal(want, got))
			}
		})
	}
}

func TestCLIActionDownload_RejectsOversizedState(t *testing.T) {
	t.Chdir(t.TempDir())
	body := &trackedCheckpointBody{Reader: bytes.NewReader(nil)}
	setDownloadTransport(t, func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/qrl/v1/debug/beacon/states/finalized", req.URL.Path)
		return &http.Response{
			StatusCode: http.StatusOK, ContentLength: beacon.MaxStateBodySize + 1,
			Body: body, Request: req,
		}, nil
	})
	require.Equal(t, int64(512<<20), beacon.MaxStateBodySize)
	require.ErrorIs(t, cliActionDownload(nil), client.ErrResponseTooLarge)
	require.Equal(t, int64(0), body.read)
	require.Equal(t, true, body.closed)
	files, err := os.ReadDir(".")
	require.NoError(t, err)
	require.Equal(t, 0, len(files))
}

func largeCheckpointFixture(t *testing.T) ([]byte, []byte) {
	t.Helper()
	const validatorCount = 4096
	cfg := params.BeaconConfig()
	// Build a structurally valid SSZ fixture without generating deposits or
	// signatures; this regression exercises downloading and serialization.
	st, err := util.NewBeaconStateZond(func(st *qrysmpb.BeaconStateZond) error {
		st.Fork.PreviousVersion = cfg.GenesisForkVersion
		st.Fork.CurrentVersion = cfg.GenesisForkVersion
		st.Validators = make([]*qrysmpb.Validator, validatorCount)
		st.Balances = make([]uint64, validatorCount)
		st.PreviousEpochParticipation = make([]byte, validatorCount)
		st.CurrentEpochParticipation = make([]byte, validatorCount)
		st.InactivityScores = make([]uint64, validatorCount)
		for i := range st.Validators {
			pubkey := make([]byte, fieldparams.MLDSA87PubkeyLength)
			binary.LittleEndian.PutUint64(pubkey, uint64(i))
			st.Validators[i] = &qrysmpb.Validator{
				PublicKey: pubkey, WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
				EffectiveBalance: cfg.MaxEffectiveBalance,
				ExitEpoch:        cfg.FarFutureEpoch, WithdrawableEpoch: cfg.FarFutureEpoch,
				RandaoCommitment: make([]byte, fieldparams.RootLength),
			}
			st.Balances[i] = cfg.MaxEffectiveBalance
		}
		return nil
	})
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	bodyRoot, err := block.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		ParentRoot: block.Block.ParentRoot, StateRoot: make([]byte, fieldparams.RootLength), BodyRoot: bodyRoot[:],
	}))
	stateRoot, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)
	block.Block.StateRoot = stateRoot[:]
	stateBytes, err := st.MarshalSSZ()
	require.NoError(t, err)
	blockBytes, err := block.MarshalSSZ()
	require.NoError(t, err)
	return stateBytes, blockBytes
}

func setDownloadTransport(t *testing.T, rt checkpointRoundTripper) {
	t.Helper()
	previousTransport, previousFlags := http.DefaultTransport, downloadFlags
	t.Cleanup(func() {
		http.DefaultTransport, downloadFlags = previousTransport, previousFlags
	})
	http.DefaultTransport = rt
	downloadFlags.BeaconNodeHost = "http://beacon.invalid"
	downloadFlags.Timeout = time.Minute
}

type checkpointRoundTripper func(*http.Request) (*http.Response, error)

func (rt checkpointRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

type trackedCheckpointBody struct {
	io.Reader
	read   int64
	closed bool
}

func (b *trackedCheckpointBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *trackedCheckpointBody) Close() error {
	b.closed = true
	return nil
}
