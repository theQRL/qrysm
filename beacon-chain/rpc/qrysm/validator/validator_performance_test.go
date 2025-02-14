package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/state"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_GetValidatorPerformance(t *testing.T) {
	t.Run("Syncing", func(t *testing.T) {
		vs := &Server{
			CoreService: &core.Service{
				SyncChecker: &mockSync.Sync{IsSyncing: true},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(vs.GetValidatorPerformance))
		req := httptest.NewRequest("POST", "/foo", nil)

		client := &http.Client{}
		rawResp, err := client.Post(srv.URL, "application/json", req.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, rawResp.StatusCode)
	})
	t.Run("Indices", func(t *testing.T) {
		ctx := context.Background()
		publicKeys := [][field_params.DilithiumPubkeyLength]byte{
			bytesutil.ToBytes2592([]byte{1}),
			bytesutil.ToBytes2592([]byte{2}),
			bytesutil.ToBytes2592([]byte{3}),
		}
		headState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		headState = setHeadState(t, headState, publicKeys)

		offset := int64(headState.Slot().Mul(params.BeaconConfig().SecondsPerSlot))
		vs := &Server{
			CoreService: &core.Service{
				HeadFetcher: &mock.ChainService{
					// 10 epochs into the future.
					State: headState,
				},
				SyncChecker:        &mockSync.Sync{IsSyncing: false},
				GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			},
		}
		c := headState.Copy()
		vp, bp, err := altair.InitializePrecomputeValidators(ctx, c)
		require.NoError(t, err)
		vp, bp, err = altair.ProcessEpochParticipation(ctx, c, bp, vp)
		require.NoError(t, err)
		c, vp, err = altair.ProcessInactivityScores(ctx, c, vp)
		require.NoError(t, err)
		_, err = altair.ProcessRewardsAndPenaltiesPrecompute(c, bp, vp)
		require.NoError(t, err)
		extraBal := params.BeaconConfig().MaxEffectiveBalance + params.BeaconConfig().GweiPerEth

		want := &ValidatorPerformanceResponse{
			PublicKeys:                    [][]byte{publicKeys[1][:], publicKeys[2][:]},
			CurrentEffectiveBalances:      []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
			CorrectlyVotedSource:          []bool{false, false},
			CorrectlyVotedTarget:          []bool{false, false},
			CorrectlyVotedHead:            []bool{false, false},
			BalancesBeforeEpochTransition: []uint64{extraBal, extraBal + params.BeaconConfig().GweiPerEth},
			BalancesAfterEpochTransition:  []uint64{vp[1].AfterEpochTransitionBalance, vp[2].AfterEpochTransitionBalance},
			MissingValidators:             [][]byte{publicKeys[0][:]},
			InactivityScores:              []uint64{0, 0},
		}
		request := &ValidatorPerformanceRequest{
			Indices: []primitives.ValidatorIndex{2, 1, 0},
		}
		var buf bytes.Buffer
		err = json.NewEncoder(&buf).Encode(request)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(vs.GetValidatorPerformance))
		req := httptest.NewRequest("POST", "/foo", &buf)
		client := &http.Client{}
		rawResp, err := client.Post(srv.URL, "application/json", req.Body)
		require.NoError(t, err)
		defer func() {
			if err := rawResp.Body.Close(); err != nil {
				t.Fatal(err)
			}
		}()
		body, err := io.ReadAll(rawResp.Body)
		require.NoError(t, err)

		response := &ValidatorPerformanceResponse{}
		require.NoError(t, json.Unmarshal(body, response))
		require.DeepEqual(t, want, response)
	})
	t.Run("Indices Pubkeys", func(t *testing.T) {
		ctx := context.Background()
		publicKeys := [][field_params.DilithiumPubkeyLength]byte{
			bytesutil.ToBytes2592([]byte{1}),
			bytesutil.ToBytes2592([]byte{2}),
			bytesutil.ToBytes2592([]byte{3}),
		}
		headState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		headState = setHeadState(t, headState, publicKeys)

		offset := int64(headState.Slot().Mul(params.BeaconConfig().SecondsPerSlot))
		vs := &Server{
			CoreService: &core.Service{
				HeadFetcher: &mock.ChainService{
					// 10 epochs into the future.
					State: headState,
				},
				SyncChecker:        &mockSync.Sync{IsSyncing: false},
				GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			},
		}
		c := headState.Copy()
		vp, bp, err := altair.InitializePrecomputeValidators(ctx, c)
		require.NoError(t, err)
		vp, bp, err = altair.ProcessEpochParticipation(ctx, c, bp, vp)
		require.NoError(t, err)
		c, vp, err = altair.ProcessInactivityScores(ctx, c, vp)
		require.NoError(t, err)
		_, err = altair.ProcessRewardsAndPenaltiesPrecompute(c, bp, vp)
		require.NoError(t, err)
		extraBal := params.BeaconConfig().MaxEffectiveBalance + params.BeaconConfig().GweiPerEth

		want := &ValidatorPerformanceResponse{
			PublicKeys:                    [][]byte{publicKeys[1][:], publicKeys[2][:]},
			CurrentEffectiveBalances:      []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
			CorrectlyVotedSource:          []bool{false, false},
			CorrectlyVotedTarget:          []bool{false, false},
			CorrectlyVotedHead:            []bool{false, false},
			BalancesBeforeEpochTransition: []uint64{extraBal, extraBal + params.BeaconConfig().GweiPerEth},
			BalancesAfterEpochTransition:  []uint64{vp[1].AfterEpochTransitionBalance, vp[2].AfterEpochTransitionBalance},
			MissingValidators:             [][]byte{publicKeys[0][:]},
			InactivityScores:              []uint64{0, 0},
		}
		request := &ValidatorPerformanceRequest{
			PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:]}, Indices: []primitives.ValidatorIndex{1, 2},
		}
		var buf bytes.Buffer
		err = json.NewEncoder(&buf).Encode(request)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(vs.GetValidatorPerformance))
		req := httptest.NewRequest("POST", "/foo", &buf)
		client := &http.Client{}
		rawResp, err := client.Post(srv.URL, "application/json", req.Body)
		require.NoError(t, err)
		defer func() {
			if err := rawResp.Body.Close(); err != nil {
				t.Fatal(err)
			}
		}()
		body, err := io.ReadAll(rawResp.Body)
		require.NoError(t, err)

		response := &ValidatorPerformanceResponse{}
		require.NoError(t, json.Unmarshal(body, response))
		require.DeepEqual(t, want, response)
	})
	t.Run("OK", func(t *testing.T) {
		helpers.ClearCache()
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MinimalSpecConfig())

		publicKeys := [][field_params.DilithiumPubkeyLength]byte{
			bytesutil.ToBytes2592([]byte{1}),
			bytesutil.ToBytes2592([]byte{2}),
			bytesutil.ToBytes2592([]byte{3}),
		}
		headState, _ := util.DeterministicGenesisStateCapella(t, 32)
		headState = setHeadState(t, headState, publicKeys)

		require.NoError(t, headState.SetInactivityScores([]uint64{0, 0, 0}))
		require.NoError(t, headState.SetBalances([]uint64{100, 101, 102}))
		offset := int64(headState.Slot().Mul(params.BeaconConfig().SecondsPerSlot))
		vs := &Server{
			CoreService: &core.Service{
				HeadFetcher: &mock.ChainService{
					State: headState,
				},
				GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
				SyncChecker:        &mockSync.Sync{IsSyncing: false},
			},
		}
		want := &ValidatorPerformanceResponse{
			PublicKeys:                    [][]byte{publicKeys[1][:], publicKeys[2][:]},
			CurrentEffectiveBalances:      []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
			CorrectlyVotedSource:          []bool{false, false},
			CorrectlyVotedTarget:          []bool{false, false},
			CorrectlyVotedHead:            []bool{false, false},
			BalancesBeforeEpochTransition: []uint64{101, 102},
			BalancesAfterEpochTransition:  []uint64{0, 0},
			MissingValidators:             [][]byte{publicKeys[0][:]},
			InactivityScores:              []uint64{0, 0},
		}
		request := &ValidatorPerformanceRequest{
			PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
		}
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(request)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(vs.GetValidatorPerformance))
		req := httptest.NewRequest("POST", "/foo", &buf)
		client := &http.Client{}
		rawResp, err := client.Post(srv.URL, "application/json", req.Body)
		require.NoError(t, err)
		defer func() {
			if err := rawResp.Body.Close(); err != nil {
				t.Fatal(err)
			}
		}()
		body, err := io.ReadAll(rawResp.Body)
		require.NoError(t, err)

		response := &ValidatorPerformanceResponse{}
		require.NoError(t, json.Unmarshal(body, response))
		require.DeepEqual(t, want, response)
	})
}

func setHeadState(t *testing.T, headState state.BeaconState, publicKeys [][field_params.DilithiumPubkeyLength]byte) state.BeaconState {
	epoch := primitives.Epoch(1)
	require.NoError(t, headState.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epoch+1))))

	defaultBal := params.BeaconConfig().MaxEffectiveBalance
	extraBal := params.BeaconConfig().MaxEffectiveBalance + params.BeaconConfig().GweiPerEth
	balances := []uint64{defaultBal, extraBal, extraBal + params.BeaconConfig().GweiPerEth}
	require.NoError(t, headState.SetBalances(balances))
	require.NoError(t, headState.SetInactivityScores([]uint64{0, 0, 0}))

	validators := []*zondpb.Validator{
		{
			PublicKey:       publicKeys[0][:],
			ActivationEpoch: 5,
			ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
		},
		{
			PublicKey:        publicKeys[1][:],
			EffectiveBalance: defaultBal,
			ActivationEpoch:  0,
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
		},
		{
			PublicKey:        publicKeys[2][:],
			EffectiveBalance: defaultBal,
			ActivationEpoch:  0,
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
		},
	}
	require.NoError(t, headState.SetValidators(validators))
	return headState
}
