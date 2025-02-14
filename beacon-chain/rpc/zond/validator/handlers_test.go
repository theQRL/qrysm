package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	dbutil "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	p2pmock "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/zond/shared"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	http2 "github.com/theQRL/qrysm/network/http"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	zondpbalpha "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestGetAggregateAttestation(t *testing.T) {
	root1 := bytesutil.PadTo([]byte("root1"), 32)
	sig1 := bytesutil.PadTo([]byte("sig1"), fieldparams.DilithiumSignatureLength)
	attSlot1 := &zondpbalpha.Attestation{
		AggregationBits: []byte{0, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            1,
			CommitteeIndex:  1,
			BeaconBlockRoot: root1,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root1,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root1,
			},
		},
		Signatures: [][]byte{sig1},
	}
	root21 := bytesutil.PadTo([]byte("root2_1"), 32)
	sig21 := bytesutil.PadTo([]byte("sig2_1"), fieldparams.DilithiumSignatureLength)
	attslot21 := &zondpbalpha.Attestation{
		AggregationBits: []byte{0, 1, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            2,
			CommitteeIndex:  2,
			BeaconBlockRoot: root21,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root21,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root21,
			},
		},
		Signatures: [][]byte{sig21},
	}
	root22 := bytesutil.PadTo([]byte("root2_2"), 32)
	sig22 := bytesutil.PadTo([]byte("sig2_2"), fieldparams.DilithiumSignatureLength)
	attslot22 := &zondpbalpha.Attestation{
		AggregationBits: []byte{0, 1, 1, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            2,
			CommitteeIndex:  3,
			BeaconBlockRoot: root22,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root22,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root22,
			},
		},
		Signatures: [][]byte{sig22},
	}
	root33 := bytesutil.PadTo([]byte("root3_3"), 32)
	sig33 := bytesutil.PadTo([]byte("sig3_3"), fieldparams.DilithiumSignatureLength)
	attslot33 := &zondpbalpha.Attestation{
		AggregationBits: []byte{1, 0, 0, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            2,
			CommitteeIndex:  3,
			BeaconBlockRoot: root33,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root33,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root33,
			},
		},
		Signatures: [][]byte{sig33},
	}
	pool := attestations.NewPool()
	err := pool.SaveAggregatedAttestations([]*zondpbalpha.Attestation{attSlot1, attslot21, attslot22})
	assert.NoError(t, err)
	s := &Server{
		AttestationsPool: pool,
	}

	t.Run("ok", func(t *testing.T) {
		reqRoot, err := attslot22.Data.HashTreeRoot()
		require.NoError(t, err)
		attDataRoot := hexutil.Encode(reqRoot[:])
		url := "http://example.com?attestation_data_root=" + attDataRoot + "&slot=2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &AggregateAttestationResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data)
		assert.DeepEqual(t, "0x00010101", resp.Data.AggregationBits)
		assert.DeepEqual(t, hexutil.Encode(sig22), resp.Data.Signatures[0])
		assert.Equal(t, "2", resp.Data.Data.Slot)
		assert.Equal(t, "3", resp.Data.Data.CommitteeIndex)
		assert.DeepEqual(t, hexutil.Encode(root22), resp.Data.Data.BeaconBlockRoot)
		require.NotNil(t, resp.Data.Data.Source)
		assert.Equal(t, "1", resp.Data.Data.Source.Epoch)
		assert.DeepEqual(t, hexutil.Encode(root22), resp.Data.Data.Source.Root)
		require.NotNil(t, resp.Data.Data.Target)
		assert.Equal(t, "1", resp.Data.Data.Target.Epoch)
		assert.DeepEqual(t, hexutil.Encode(root22), resp.Data.Data.Target.Root)
	})

	t.Run("aggregate beforehand", func(t *testing.T) {
		err = s.AttestationsPool.SaveUnaggregatedAttestation(attslot33)
		require.NoError(t, err)
		newAtt := zondpbalpha.CopyAttestation(attslot33)
		newAtt.AggregationBits = []byte{0, 1, 0, 1}
		err = s.AttestationsPool.SaveUnaggregatedAttestation(newAtt)
		require.NoError(t, err)

		reqRoot, err := attslot33.Data.HashTreeRoot()
		require.NoError(t, err)
		attDataRoot := hexutil.Encode(reqRoot[:])
		url := "http://example.com?attestation_data_root=" + attDataRoot + "&slot=2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &AggregateAttestationResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		assert.DeepEqual(t, "0x01010001", resp.Data.AggregationBits)
	})
	t.Run("no matching attestation", func(t *testing.T) {
		attDataRoot := hexutil.Encode(bytesutil.PadTo([]byte("foo"), 32))
		url := "http://example.com?attestation_data_root=" + attDataRoot + "&slot=2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No matching attestation found"))
	})
	t.Run("no attestation_data_root provided", func(t *testing.T) {
		url := "http://example.com?slot=2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Attestation data root is required"))
	})
	t.Run("invalid attestation_data_root provided", func(t *testing.T) {
		url := "http://example.com?attestation_data_root=foo&slot=2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Attestation data root is invalid"))
	})
	t.Run("no slot provided", func(t *testing.T) {
		attDataRoot := hexutil.Encode(bytesutil.PadTo([]byte("foo"), 32))
		url := "http://example.com?attestation_data_root=" + attDataRoot
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Slot is required"))
	})
	t.Run("invalid slot provided", func(t *testing.T) {
		attDataRoot := hexutil.Encode(bytesutil.PadTo([]byte("foo"), 32))
		url := "http://example.com?attestation_data_root=" + attDataRoot + "&slot=foo"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAggregateAttestation(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Slot is invalid"))
	})
}

func TestGetAggregateAttestation_SameSlotAndRoot_ReturnMostAggregationBits(t *testing.T) {
	root := bytesutil.PadTo([]byte("root"), 32)
	sig := bytesutil.PadTo([]byte("sig"), fieldparams.DilithiumSignatureLength)
	att1 := &zondpbalpha.Attestation{
		AggregationBits: []byte{3, 0, 0, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            1,
			CommitteeIndex:  1,
			BeaconBlockRoot: root,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
		},
		Signatures: [][]byte{sig},
	}
	att2 := &zondpbalpha.Attestation{
		AggregationBits: []byte{0, 3, 0, 1},
		Data: &zondpbalpha.AttestationData{
			Slot:            1,
			CommitteeIndex:  1,
			BeaconBlockRoot: root,
			Source: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
			Target: &zondpbalpha.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
		},
		Signatures: [][]byte{sig},
	}
	pool := attestations.NewPool()
	err := pool.SaveAggregatedAttestations([]*zondpbalpha.Attestation{att1, att2})
	assert.NoError(t, err)
	s := &Server{
		AttestationsPool: pool,
	}
	reqRoot, err := att1.Data.HashTreeRoot()
	require.NoError(t, err)
	attDataRoot := hexutil.Encode(reqRoot[:])
	url := "http://example.com?attestation_data_root=" + attDataRoot + "&slot=1"
	request := httptest.NewRequest(http.MethodGet, url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetAggregateAttestation(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &AggregateAttestationResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.NotNil(t, resp)
	assert.DeepEqual(t, "0x03000001", resp.Data.AggregationBits)
}

func TestSubmitContributionAndProofs(t *testing.T) {
	c := &core.Service{
		OperationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
	}

	s := &Server{CoreService: c}

	t.Run("single", func(t *testing.T) {
		broadcaster := &p2pmock.MockBroadcaster{}
		c.Broadcaster = broadcaster
		c.SyncCommitteePool = synccommittee.NewStore()

		var body bytes.Buffer
		_, err := body.WriteString(singleContribution)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitContributionAndProofs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, 1, len(broadcaster.BroadcastMessages))
		contributions, err := c.SyncCommitteePool.SyncCommitteeContributions(1)
		require.NoError(t, err)
		assert.Equal(t, 1, len(contributions))
	})
	t.Run("multiple", func(t *testing.T) {
		broadcaster := &p2pmock.MockBroadcaster{}
		c.Broadcaster = broadcaster
		c.SyncCommitteePool = synccommittee.NewStore()

		var body bytes.Buffer
		_, err := body.WriteString(multipleContributions)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitContributionAndProofs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, 2, len(broadcaster.BroadcastMessages))
		contributions, err := c.SyncCommitteePool.SyncCommitteeContributions(1)
		require.NoError(t, err)
		assert.Equal(t, 2, len(contributions))
	})
	t.Run("no body", func(t *testing.T) {
		s.SyncCommitteePool = synccommittee.NewStore()

		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitContributionAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitContributionAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("invalid", func(t *testing.T) {
		c.SyncCommitteePool = synccommittee.NewStore()

		var body bytes.Buffer
		_, err := body.WriteString(invalidContribution)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitContributionAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
}

func TestSubmitAggregateAndProofs(t *testing.T) {
	c := &core.Service{
		GenesisTimeFetcher: &mockChain.ChainService{},
	}

	s := &Server{
		CoreService: c,
	}

	t.Run("single", func(t *testing.T) {
		broadcaster := &p2pmock.MockBroadcaster{}
		c.Broadcaster = broadcaster

		var body bytes.Buffer
		_, err := body.WriteString(singleAggregate)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitAggregateAndProofs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, 1, len(broadcaster.BroadcastMessages))
	})
	t.Run("multiple", func(t *testing.T) {
		broadcaster := &p2pmock.MockBroadcaster{}
		c.Broadcaster = broadcaster
		c.SyncCommitteePool = synccommittee.NewStore()

		var body bytes.Buffer
		_, err := body.WriteString(multipleAggregates)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitAggregateAndProofs(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, 2, len(broadcaster.BroadcastMessages))
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitAggregateAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitAggregateAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("invalid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(invalidAggregate)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitAggregateAndProofs(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
}

func TestSubmitSyncCommitteeSubscription(t *testing.T) {
	genesis := util.NewBeaconBlockCapella()
	deposits, _, err := util.DeterministicDepositsAndKeys(64)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data)
	require.NoError(t, err, "Could not set up genesis state")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	roots := make([][]byte, fieldparams.BlockRootsLength)
	roots[0] = genesisRoot[:]
	require.NoError(t, bs.SetBlockRoots(roots))

	pubkeys := make([][]byte, len(deposits))
	for i := 0; i < len(deposits); i++ {
		pubkeys[i] = deposits[i].Data.PublicKey
	}

	chainSlot := primitives.Slot(0)
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Slot: &chainSlot,
	}
	s := &Server{
		HeadFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	t.Run("single", func(t *testing.T) {
		cache.SyncSubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(singleSyncCommitteeSubscription)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets, _, _, _ := cache.SyncSubnetIDs.GetSyncCommitteeSubnets(pubkeys[1], 0)
		require.Equal(t, 2, len(subnets))
		assert.Equal(t, uint64(0), subnets[0])
		assert.Equal(t, uint64(2), subnets[1])
	})
	t.Run("multiple", func(t *testing.T) {
		cache.SyncSubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(multipleSyncCommitteeSubscription)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets, _, _, _ := cache.SyncSubnetIDs.GetSyncCommitteeSubnets(pubkeys[0], 0)
		require.Equal(t, 1, len(subnets))
		assert.Equal(t, uint64(0), subnets[0])
		subnets, _, _, _ = cache.SyncSubnetIDs.GetSyncCommitteeSubnets(pubkeys[1], 0)
		require.Equal(t, 1, len(subnets))
		assert.Equal(t, uint64(2), subnets[0])
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("invalid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(invalidSyncCommitteeSubscription)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
	t.Run("epoch in the past", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(singleSyncCommitteeSubscription2)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Epoch for subscription at index 0 is in the past"))
	})
	t.Run("first epoch after the next sync committee is valid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(singleSyncCommitteeSubscription3)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("epoch too far in the future", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(singleSyncCommitteeSubscription4)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Epoch for subscription at index 0 is too far in the future"))
	})
	t.Run("sync not ready", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chainService := &mockChain.ChainService{State: st}
		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitSyncCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Beacon node is currently syncing"))
	})
}

func TestSubmitBeaconCommitteeSubscription(t *testing.T) {
	genesis := util.NewBeaconBlockCapella()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
	require.NoError(t, err, "Could not set up genesis state")
	// Set state to non-epoch start slot.
	require.NoError(t, bs.SetSlot(5))
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	roots := make([][]byte, fieldparams.BlockRootsLength)
	roots[0] = genesisRoot[:]
	require.NoError(t, bs.SetBlockRoots(roots))

	pubkeys := make([][]byte, len(deposits))
	for i := 0; i < len(deposits); i++ {
		pubkeys[i] = deposits[i].Data.PublicKey
	}

	chainSlot := primitives.Slot(0)
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Slot: &chainSlot,
	}
	s := &Server{
		HeadFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	t.Run("single", func(t *testing.T) {
		cache.SubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(singleBeaconCommitteeContribution)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets := cache.SubnetIDs.GetAttesterSubnetIDs(1)
		require.Equal(t, 1, len(subnets))
		assert.Equal(t, uint64(2), subnets[0])
	})
	t.Run("multiple", func(t *testing.T) {
		cache.SubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(multipleBeaconCommitteeContribution)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets := cache.SubnetIDs.GetAttesterSubnetIDs(1)
		require.Equal(t, 2, len(subnets))
		assert.Equal(t, uint64(2), subnets[0])
		assert.Equal(t, uint64(1), subnets[1])
	})
	t.Run("is aggregator", func(t *testing.T) {
		cache.SubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(singleBeaconCommitteeContribution2)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets := cache.SubnetIDs.GetAggregatorSubnetIDs(1)
		require.Equal(t, 1, len(subnets))
		assert.Equal(t, uint64(2), subnets[0])
	})
	t.Run("validators assigned to subnets", func(t *testing.T) {
		cache.SubnetIDs.EmptyAllCaches()

		var body bytes.Buffer
		_, err := body.WriteString(multipleBeaconCommitteeContribution2)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		subnets, ok, _ := cache.SubnetIDs.GetPersistentSubnets(pubkeys[1])
		require.Equal(t, true, ok, "subnet for validator 1 not found")
		assert.Equal(t, 1, len(subnets))
		subnets, ok, _ = cache.SubnetIDs.GetPersistentSubnets(pubkeys[2])
		require.Equal(t, true, ok, "subnet for validator 2 not found")
		assert.Equal(t, 1, len(subnets))
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "No data submitted"))
	})
	t.Run("invalid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(invalidBeaconCommitteeContribution)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
	t.Run("sync not ready", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chainService := &mockChain.ChainService{State: st}
		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.SubmitBeaconCommitteeSubscription(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Beacon node is currently syncing"))
	})
}

func TestGetAttestationData(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		block := util.NewBeaconBlockCapella()
		block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
		targetBlock := util.NewBeaconBlockCapella()
		targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
		justifiedBlock := util.NewBeaconBlockCapella()
		justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
		blockRoot, err := block.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash beacon block")
		justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for justified block")
		targetRoot, err := targetBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for target block")
		slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(slot))
		err = beaconState.SetCurrentJustifiedCheckpoint(&zondpbalpha.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		})
		require.NoError(t, err)

		blockRoots := beaconState.BlockRoots()
		blockRoots[1] = blockRoot[:]
		blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
		blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
		require.NoError(t, beaconState.SetBlockRoots(blockRoots))
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		chain := &mockChain.ChainService{
			Optimistic: false,
			Genesis:    time.Now().Add(time.Duration(-1*offset) * time.Second),
			State:      beaconState,
			Root:       blockRoot[:],
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				HeadFetcher:        chain,
				GenesisTimeFetcher: chain,
			},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", slot, 0)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		expectedResponse := &GetAttestationDataResponse{
			Data: &shared.AttestationData{
				Slot:            strconv.FormatUint(uint64(slot), 10),
				BeaconBlockRoot: hexutil.Encode(blockRoot[:]),
				CommitteeIndex:  strconv.FormatUint(0, 10),
				Source: &shared.Checkpoint{
					Epoch: strconv.FormatUint(2, 10),
					Root:  hexutil.Encode(justifiedRoot[:]),
				},
				Target: &shared.Checkpoint{
					Epoch: strconv.FormatUint(3, 10),
					Root:  hexutil.Encode(blockRoot[:]),
				},
			},
		}

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		assert.DeepEqual(t, expectedResponse, resp)
	})

	t.Run("syncing", func(t *testing.T) {
		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chain := &mockChain.ChainService{
			Optimistic: false,
			State:      beaconState,
			Genesis:    time.Now(),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", 1, 2)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "syncing"))
	})

	t.Run("optimistic", func(t *testing.T) {
		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chain := &mockChain.ChainService{
			Optimistic: true,
			State:      beaconState,
			Genesis:    time.Now(),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				GenesisTimeFetcher: chain,
				HeadFetcher:        chain,
			},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", 0, 0)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "optimistic"))

		chain.Optimistic = false

		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		assert.Equal(t, http.StatusOK, writer.Code)
	})

	t.Run("handles in progress request", func(t *testing.T) {
		state, err := state_native.InitializeFromProtoCapella(&zondpbalpha.BeaconStateCapella{Slot: 100})
		require.NoError(t, err)
		ctx := context.Background()
		slot := primitives.Slot(2)
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		chain := &mockChain.ChainService{
			Optimistic: false,
			Genesis:    time.Now().Add(time.Duration(-1*offset) * time.Second),
			State:      state,
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				HeadFetcher:        chain,
				GenesisTimeFetcher: chain,
			},
		}

		expectedResponse := &GetAttestationDataResponse{
			Data: &shared.AttestationData{
				Slot:            strconv.FormatUint(uint64(slot), 10),
				CommitteeIndex:  strconv.FormatUint(1, 10),
				BeaconBlockRoot: hexutil.Encode(make([]byte, 32)),
				Source: &shared.Checkpoint{
					Epoch: strconv.FormatUint(42, 10),
					Root:  hexutil.Encode(make([]byte, 32)),
				},
				Target: &shared.Checkpoint{
					Epoch: strconv.FormatUint(55, 10),
					Root:  hexutil.Encode(make([]byte, 32)),
				},
			},
		}

		expectedResponsePb := &zondpbalpha.AttestationData{
			Slot:            slot,
			CommitteeIndex:  1,
			BeaconBlockRoot: make([]byte, 32),
			Source:          &zondpbalpha.Checkpoint{Epoch: 42, Root: make([]byte, 32)},
			Target:          &zondpbalpha.Checkpoint{Epoch: 55, Root: make([]byte, 32)},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", slot, 1)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		requestPb := &zondpbalpha.AttestationDataRequest{
			CommitteeIndex: 1,
			Slot:           slot,
		}

		require.NoError(t, s.CoreService.AttestationCache.MarkInProgress(requestPb))

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetAttestationData(writer, request)

			assert.Equal(t, http.StatusOK, writer.Code)
			resp := &GetAttestationDataResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.NotNil(t, resp)
			assert.DeepEqual(t, expectedResponse, resp)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()

			assert.NoError(t, s.CoreService.AttestationCache.Put(ctx, requestPb, expectedResponsePb))
			assert.NoError(t, s.CoreService.AttestationCache.MarkNotInProgress(requestPb))
		}()

		wg.Wait()
	})

	t.Run("invalid slot", func(t *testing.T) {
		slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		chain := &mockChain.ChainService{
			Optimistic: false,
			Genesis:    time.Now().Add(time.Duration(-1*offset) * time.Second),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				GenesisTimeFetcher: chain,
			},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", 1000000000000, 2)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "invalid request"))
	})

	t.Run("head state slot greater than request slot", func(t *testing.T) {
		ctx := context.Background()
		db := dbutil.SetupDB(t)

		slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
		block := util.NewBeaconBlockCapella()
		block.Block.Slot = slot
		block2 := util.NewBeaconBlockCapella()
		block2.Block.Slot = slot - 1
		targetBlock := util.NewBeaconBlockCapella()
		targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
		justifiedBlock := util.NewBeaconBlockCapella()
		justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
		blockRoot, err := block.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash beacon block")
		blockRoot2, err := block2.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, block2)
		justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for justified block")
		targetRoot, err := targetBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for target block")

		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(slot))
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix()-offset)))
		err = beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpbalpha.BeaconBlockHeader{
			ParentRoot: blockRoot2[:],
		}))
		require.NoError(t, err)
		err = beaconState.SetCurrentJustifiedCheckpoint(&zondpbalpha.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		})
		require.NoError(t, err)
		blockRoots := beaconState.BlockRoots()
		blockRoots[1] = blockRoot[:]
		blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
		blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
		blockRoots[3*params.BeaconConfig().SlotsPerEpoch] = blockRoot2[:]
		require.NoError(t, beaconState.SetBlockRoots(blockRoots))

		beaconstate := beaconState.Copy()
		require.NoError(t, beaconstate.SetSlot(beaconstate.Slot()-1))
		require.NoError(t, db.SaveState(ctx, beaconstate, blockRoot2))
		chain := &mockChain.ChainService{
			State:   beaconState,
			Root:    blockRoot[:],
			Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				HeadFetcher:        chain,
				GenesisTimeFetcher: chain,
				StateGen:           stategen.New(db, doublylinkedtree.New()),
			},
		}

		require.NoError(t, db.SaveState(ctx, beaconState, blockRoot))
		util.SaveBlock(t, ctx, db, block)
		require.NoError(t, db.SaveHeadBlockRoot(ctx, blockRoot))

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", slot-1, 0)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		expectedResponse := &GetAttestationDataResponse{
			Data: &shared.AttestationData{
				Slot:            strconv.FormatUint(uint64(slot-1), 10),
				CommitteeIndex:  strconv.FormatUint(0, 10),
				BeaconBlockRoot: hexutil.Encode(blockRoot2[:]),
				Source: &shared.Checkpoint{
					Epoch: strconv.FormatUint(2, 10),
					Root:  hexutil.Encode(justifiedRoot[:]),
				},
				Target: &shared.Checkpoint{
					Epoch: strconv.FormatUint(3, 10),
					Root:  hexutil.Encode(blockRoot2[:]),
				},
			},
		}

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		assert.DeepEqual(t, expectedResponse, resp)
	})

	t.Run("succeeds in first epoch", func(t *testing.T) {
		slot := primitives.Slot(5)
		block := util.NewBeaconBlockCapella()
		block.Block.Slot = slot
		targetBlock := util.NewBeaconBlockCapella()
		targetBlock.Block.Slot = 0
		justifiedBlock := util.NewBeaconBlockCapella()
		justifiedBlock.Block.Slot = 0
		blockRoot, err := block.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash beacon block")
		justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for justified block")
		targetRoot, err := targetBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root for target block")

		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(slot))
		err = beaconState.SetCurrentJustifiedCheckpoint(&zondpbalpha.Checkpoint{
			Epoch: 0,
			Root:  justifiedRoot[:],
		})
		require.NoError(t, err)
		blockRoots := beaconState.BlockRoots()
		blockRoots[1] = blockRoot[:]
		blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
		blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
		require.NoError(t, beaconState.SetBlockRoots(blockRoots))
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		chain := &mockChain.ChainService{
			State:   beaconState,
			Root:    blockRoot[:],
			Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				HeadFetcher:        chain,
				GenesisTimeFetcher: chain,
			},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", slot, 0)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		expectedResponse := &GetAttestationDataResponse{
			Data: &shared.AttestationData{
				Slot:            strconv.FormatUint(uint64(slot), 10),
				BeaconBlockRoot: hexutil.Encode(blockRoot[:]),
				CommitteeIndex:  strconv.FormatUint(0, 10),
				Source: &shared.Checkpoint{
					Epoch: strconv.FormatUint(0, 10),
					Root:  hexutil.Encode(justifiedRoot[:]),
				},
				Target: &shared.Checkpoint{
					Epoch: strconv.FormatUint(0, 10),
					Root:  hexutil.Encode(blockRoot[:]),
				},
			},
		}

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		assert.DeepEqual(t, expectedResponse, resp)
	})

	t.Run("handles far away justified epoch", func(t *testing.T) {
		// Scenario:
		//
		// State slot = 40000
		// Last justified slot = epoch start of 1500
		// HistoricalRootsLimit = 8192
		//
		// More background: https://github.com/prysmaticlabs/prysm/issues/2153
		// This test breaks if it doesn't use mainnet config

		// Ensure HistoricalRootsLimit matches scenario
		params.SetupTestConfigCleanup(t)
		cfg := params.MainnetConfig().Copy()
		cfg.HistoricalRootsLimit = 8192
		params.OverrideBeaconConfig(cfg)

		block := util.NewBeaconBlockCapella()
		block.Block.Slot = 40000
		epochBoundaryBlock := util.NewBeaconBlockCapella()
		var err error
		epochBoundaryBlock.Block.Slot, err = slots.EpochStart(slots.ToEpoch(40000))
		require.NoError(t, err)
		justifiedBlock := util.NewBeaconBlockCapella()
		justifiedBlock.Block.Slot, err = slots.EpochStart(slots.ToEpoch(1500))
		require.NoError(t, err)
		justifiedBlock.Block.Slot -= 2 // Imagine two skip block
		blockRoot, err := block.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash beacon block")
		justifiedBlockRoot, err := justifiedBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash justified block")
		epochBoundaryRoot, err := epochBoundaryBlock.Block.HashTreeRoot()
		require.NoError(t, err, "Could not hash justified block")
		slot := primitives.Slot(40000)

		beaconState, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(slot))
		err = beaconState.SetCurrentJustifiedCheckpoint(&zondpbalpha.Checkpoint{
			Epoch: slots.ToEpoch(1500),
			Root:  justifiedBlockRoot[:],
		})
		require.NoError(t, err)
		blockRoots := beaconState.BlockRoots()
		blockRoots[1] = blockRoot[:]
		blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = epochBoundaryRoot[:]
		blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedBlockRoot[:]
		require.NoError(t, beaconState.SetBlockRoots(blockRoots))
		offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
		chain := &mockChain.ChainService{
			State:   beaconState,
			Root:    blockRoot[:],
			Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
		}

		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			CoreService: &core.Service{
				AttestationCache:   cache.NewAttestationCache(),
				HeadFetcher:        chain,
				GenesisTimeFetcher: chain,
			},
		}

		url := fmt.Sprintf("http://example.com?slot=%d&committee_index=%d", slot, 0)
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttestationData(writer, request)

		expectedResponse := &GetAttestationDataResponse{
			Data: &shared.AttestationData{
				Slot:            strconv.FormatUint(uint64(slot), 10),
				BeaconBlockRoot: hexutil.Encode(blockRoot[:]),
				CommitteeIndex:  strconv.FormatUint(0, 10),
				Source: &shared.Checkpoint{
					Epoch: strconv.FormatUint(uint64(slots.ToEpoch(1500)), 10),
					Root:  hexutil.Encode(justifiedBlockRoot[:]),
				},
				Target: &shared.Checkpoint{
					Epoch: strconv.FormatUint(312, 10),
					Root:  hexutil.Encode(blockRoot[:]),
				},
			},
		}

		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		assert.DeepEqual(t, expectedResponse, resp)
	})
}

func TestProduceSyncCommitteeContribution(t *testing.T) {
	root := bytesutil.PadTo([]byte("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"), 32)
	sig := []byte{}
	messsage := &zondpbalpha.SyncCommitteeMessage{
		Slot:           1,
		BlockRoot:      root,
		ValidatorIndex: 0,
		Signature:      sig,
	}
	syncCommitteePool := synccommittee.NewStore()
	require.NoError(t, syncCommitteePool.SaveSyncCommitteeMessage(messsage))
	server := Server{
		CoreService: &core.Service{
			HeadFetcher: &mockChain.ChainService{
				SyncCommitteeIndices: []primitives.CommitteeIndex{0},
			},
		},
		SyncCommitteePool: syncCommitteePool,
	}
	t.Run("ok", func(t *testing.T) {
		url := "http://example.com?slot=1&subcommittee_index=1&beacon_block_root=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		require.Equal(t, resp.Data.Slot, "1")
		require.Equal(t, resp.Data.SubcommitteeIndex, "1")
		require.Equal(t, resp.Data.BeaconBlockRoot, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	})
	t.Run("no slot provided", func(t *testing.T) {
		url := "http://example.com?subcommittee_index=1&beacon_block_root=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.ErrorContains(t, "Slot is required", errors.New(writer.Body.String()))
	})
	t.Run("no subcommittee_index provided", func(t *testing.T) {
		url := "http://example.com?slot=1&beacon_block_root=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.ErrorContains(t, "Subcommittee Index is required", errors.New(writer.Body.String()))
	})
	t.Run("no beacon_block_root provided", func(t *testing.T) {
		url := "http://example.com?slot=1&subcommittee_index=1"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.ErrorContains(t, "Invalid Beacon Block Root: empty hex string", errors.New(writer.Body.String()))
	})
	t.Run("invalid block root", func(t *testing.T) {
		url := "http://example.com?slot=1&subcommittee_index=1&beacon_block_root=0"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.ErrorContains(t, "Invalid Beacon Block Root: hex string without 0x prefix", errors.New(writer.Body.String()))
	})
	t.Run("no committee messages", func(t *testing.T) {
		url := "http://example.com?slot=1&subcommittee_index=1&beacon_block_root=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data)

		request = httptest.NewRequest(http.MethodGet, url, nil)
		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		syncCommitteePool = synccommittee.NewStore()
		server = Server{
			CoreService: &core.Service{
				HeadFetcher: &mockChain.ChainService{
					SyncCommitteeIndices: []primitives.CommitteeIndex{0},
				},
			},
			SyncCommitteePool: syncCommitteePool,
		}
		server.ProduceSyncCommitteeContribution(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		resp2 := &ProduceSyncCommitteeContributionResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp2))
		require.ErrorContains(t, "No subcommittee messages found", errors.New(writer.Body.String()))
	})
}

func TestServer_RegisterValidator(t *testing.T) {

	tests := []struct {
		name    string
		request string
		code    int
		wantErr string
	}{
		{
			name:    "Happy Path",
			request: registrations,
			code:    http.StatusOK,
			wantErr: "",
		},
		{
			name:    "Empty Request",
			request: "",
			code:    http.StatusBadRequest,
			wantErr: "No data submitted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			_, err := body.WriteString(tt.request)
			require.NoError(t, err)
			url := "http://example.com/eth/v1/validator/register_validator"
			request := httptest.NewRequest(http.MethodPost, url, &body)
			writer := httptest.NewRecorder()
			db := dbutil.SetupDB(t)

			server := Server{
				CoreService: &core.Service{
					HeadFetcher: &mockChain.ChainService{
						SyncCommitteeIndices: []primitives.CommitteeIndex{0},
					},
				},
				BlockBuilder: &builderTest.MockBuilderService{
					HasConfigured: true,
				},
				BeaconDB: db,
			}

			server.RegisterValidator(writer, request)
			require.Equal(t, tt.code, writer.Code)
			if tt.wantErr != "" {
				require.Equal(t, strings.Contains(writer.Body.String(), tt.wantErr), true)
			}
		})
	}
}

func TestGetAttesterDuties(t *testing.T) {
	helpers.ClearCache()

	genesis := util.NewBeaconBlockCapella()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
	require.NoError(t, err, "Could not set up genesis state")
	// Set state to non-epoch start slot.
	require.NoError(t, bs.SetSlot(5))
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	roots := make([][]byte, fieldparams.BlockRootsLength)
	roots[0] = genesisRoot[:]
	require.NoError(t, bs.SetBlockRoots(roots))

	// Deactivate last validator.
	vals := bs.Validators()
	vals[len(vals)-1].ExitEpoch = 0
	require.NoError(t, bs.SetValidators(vals))

	pubKeys := make([][]byte, len(deposits))
	for i := 0; i < len(deposits); i++ {
		pubKeys[i] = deposits[i].Data.PublicKey
	}

	// nextEpochState must not be used for committee calculations when requesting next epoch
	nextEpochState := bs.Copy()
	require.NoError(t, nextEpochState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, nextEpochState.SetValidators(vals[:512]))

	chainSlot := primitives.Slot(0)
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Slot: &chainSlot,
	}
	s := &Server{
		Stater: &testutil.MockStater{
			StatesBySlot: map[primitives.Slot]state.BeaconState{
				0:                                   bs,
				params.BeaconConfig().SlotsPerEpoch: nextEpochState,
			},
		},
		TimeFetcher:           chain,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: chain,
	}

	t.Run("single validator", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString("[\"0\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttesterDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(genesisRoot[:]), resp.DependentRoot)
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, "0", duty.CommitteeIndex)
		assert.Equal(t, "46", duty.Slot)
		assert.Equal(t, hexutil.Encode(pubKeys[0]), duty.Pubkey)
		assert.Equal(t, "128", duty.CommitteeLength)
		assert.Equal(t, "1", duty.CommitteesAtSlot)
		assert.Equal(t, "66", duty.ValidatorCommitteeIndex)
	})
	t.Run("multiple validators", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttesterDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("invalid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"foo\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
	t.Run("next epoch", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString("[\"0\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(slots.ToEpoch(bs.Slot())+1), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttesterDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(genesisRoot[:]), resp.DependentRoot)
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, "0", duty.CommitteeIndex)
		assert.Equal(t, "133", duty.Slot)
		assert.Equal(t, "0", duty.ValidatorIndex)
		assert.Equal(t, hexutil.Encode(pubKeys[0]), duty.Pubkey)
		assert.Equal(t, "128", duty.CommitteeLength)
		assert.Equal(t, "1", duty.CommitteesAtSlot)
		assert.Equal(t, "103", duty.ValidatorCommitteeIndex)
	})
	t.Run("epoch out of bounds", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString("[\"0\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		currentEpoch := slots.ToEpoch(bs.Slot())
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(currentEpoch+2), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", currentEpoch+2, currentEpoch+1)))
	})
	t.Run("validator index out of bounds", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString(fmt.Sprintf("[\"%d\"]", len(pubKeys)))
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, fmt.Sprintf("Invalid validator index %d", len(pubKeys))))
	})
	t.Run("inactive validator - no duties", func(t *testing.T) {
		var body bytes.Buffer
		_, err = body.WriteString(fmt.Sprintf("[\"%d\"]", len(pubKeys)-1))
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttesterDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 0, len(resp.Data))
	})
	t.Run("execution optimistic", func(t *testing.T) {
		ctx := context.Background()

		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		blk.Block.Slot = 31
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		db := dbutil.SetupDB(t)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainSlot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot, Optimistic: true,
		}
		s := &Server{
			Stater:                &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{0: bs}},
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
		}

		var body bytes.Buffer
		_, err = body.WriteString("[\"0\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &GetAttesterDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("sync not ready", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chainService := &mockChain.ChainService{State: st}
		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/attester/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAttesterDuties(writer, request)
		require.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
	})
}

func TestGetProposerDuties(t *testing.T) {
	helpers.ClearCache()

	genesis := util.NewBeaconBlockCapella()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	roots := make([][]byte, fieldparams.BlockRootsLength)
	roots[0] = genesisRoot[:]
	// We DON'T WANT this root to be returned when testing the next epoch
	roots[31] = []byte("next_epoch_dependent_root")

	pubKeys := make([][]byte, len(deposits))
	for i := 0; i < len(deposits); i++ {
		pubKeys[i] = deposits[i].Data.PublicKey
	}

	t.Run("ok", func(t *testing.T) {
		bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
		require.NoError(t, err, "Could not set up genesis state")
		require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch))
		require.NoError(t, bs.SetBlockRoots(roots))
		chainSlot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot,
		}
		s := &Server{
			Stater:                 &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{0: bs}},
			HeadFetcher:            chain,
			TimeFetcher:            chain,
			OptimisticModeFetcher:  chain,
			SyncChecker:            &mockSync.Sync{IsSyncing: false},
			ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetProposerDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(genesisRoot[:]), resp.DependentRoot)
		assert.Equal(t, 127, len(resp.Data))
		// We expect a proposer duty for slot 11.
		var expectedDuty *ProposerDuty
		for _, duty := range resp.Data {
			if duty.Slot == "11" {
				expectedDuty = duty
			}
		}
		vid, _, has := s.ProposerSlotIndexCache.GetProposerPayloadIDs(11, [32]byte{})
		require.Equal(t, true, has)
		require.Equal(t, primitives.ValidatorIndex(754), vid)
		require.NotNil(t, expectedDuty, "Expected duty for slot 11 not found")
		assert.Equal(t, "754", expectedDuty.ValidatorIndex)
		assert.Equal(t, hexutil.Encode(pubKeys[754]), expectedDuty.Pubkey)
	})
	t.Run("next epoch", func(t *testing.T) {
		bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
		require.NoError(t, err, "Could not set up genesis state")
		require.NoError(t, bs.SetBlockRoots(roots))
		chainSlot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot,
		}
		s := &Server{
			Stater:                 &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{0: bs}},
			HeadFetcher:            chain,
			TimeFetcher:            chain,
			OptimisticModeFetcher:  chain,
			SyncChecker:            &mockSync.Sync{IsSyncing: false},
			ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetProposerDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(genesisRoot[:]), resp.DependentRoot)
		assert.Equal(t, 128, len(resp.Data))
		// We expect a proposer duty for slot 139.
		var expectedDuty *ProposerDuty
		for _, duty := range resp.Data {
			if duty.Slot == "139" {
				expectedDuty = duty
			}
		}
		vid, _, has := s.ProposerSlotIndexCache.GetProposerPayloadIDs(139, [32]byte{})
		require.Equal(t, true, has)
		require.Equal(t, primitives.ValidatorIndex(10462), vid)
		require.NotNil(t, expectedDuty, "Expected duty for slot 139 not found")
		assert.Equal(t, "10462", expectedDuty.ValidatorIndex)
		assert.Equal(t, hexutil.Encode(pubKeys[10462]), expectedDuty.Pubkey)
	})
	t.Run("prune payload ID cache", func(t *testing.T) {
		bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
		require.NoError(t, err, "Could not set up genesis state")
		require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch))
		require.NoError(t, bs.SetBlockRoots(roots))
		chainSlot := params.BeaconConfig().SlotsPerEpoch
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot,
		}
		s := &Server{
			Stater:                 &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{params.BeaconConfig().SlotsPerEpoch: bs}},
			HeadFetcher:            chain,
			TimeFetcher:            chain,
			OptimisticModeFetcher:  chain,
			SyncChecker:            &mockSync.Sync{IsSyncing: false},
			ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		}

		s.ProposerSlotIndexCache.SetProposerAndPayloadIDs(1, 1, [8]byte{1}, [32]byte{2})
		s.ProposerSlotIndexCache.SetProposerAndPayloadIDs(31, 2, [8]byte{2}, [32]byte{3})
		s.ProposerSlotIndexCache.SetProposerAndPayloadIDs(32, 4309, [8]byte{3}, [32]byte{4})

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/zond/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		vid, _, has := s.ProposerSlotIndexCache.GetProposerPayloadIDs(1, [32]byte{})
		require.Equal(t, false, has)
		require.Equal(t, primitives.ValidatorIndex(0), vid)
		vid, _, has = s.ProposerSlotIndexCache.GetProposerPayloadIDs(2, [32]byte{})
		require.Equal(t, false, has)
		require.Equal(t, primitives.ValidatorIndex(0), vid)
		vid, _, has = s.ProposerSlotIndexCache.GetProposerPayloadIDs(128, [32]byte{})
		require.Equal(t, true, has)
		require.Equal(t, primitives.ValidatorIndex(14916), vid)
	})
	t.Run("epoch out of bounds", func(t *testing.T) {
		bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
		require.NoError(t, err, "Could not set up genesis state")
		// Set state to non-epoch start slot.
		require.NoError(t, bs.SetSlot(5))
		require.NoError(t, bs.SetBlockRoots(roots))
		chainSlot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot,
		}
		s := &Server{
			Stater:                 &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{0: bs}},
			HeadFetcher:            chain,
			TimeFetcher:            chain,
			OptimisticModeFetcher:  chain,
			SyncChecker:            &mockSync.Sync{IsSyncing: false},
			ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		}

		currentEpoch := slots.ToEpoch(bs.Slot())
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(currentEpoch+2), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", currentEpoch+2, currentEpoch+1), e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		ctx := context.Background()
		bs, err := transition.GenesisBeaconStateCapella(context.Background(), deposits, 0, eth1Data, &enginev1.ExecutionPayloadCapella{})
		require.NoError(t, err, "Could not set up genesis state")
		// Set state to non-epoch start slot.
		require.NoError(t, bs.SetSlot(5))
		require.NoError(t, bs.SetBlockRoots(roots))
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		blk.Block.Slot = 127
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		db := dbutil.SetupDB(t)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainSlot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			State: bs, Root: genesisRoot[:], Slot: &chainSlot, Optimistic: true,
		}
		s := &Server{
			Stater:                 &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{0: bs}},
			HeadFetcher:            chain,
			TimeFetcher:            chain,
			OptimisticModeFetcher:  chain,
			SyncChecker:            &mockSync.Sync{IsSyncing: false},
			ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetProposerDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("sync not ready", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chainService := &mockChain.ChainService{State: st}
		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/proposer/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetProposerDuties(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
	})
}

func TestGetSyncCommitteeDuties(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)

	genesisTime := time.Now()
	numVals := uint64(11)
	st, _ := util.DeterministicGenesisStateCapella(t, numVals)
	require.NoError(t, st.SetGenesisTime(uint64(genesisTime.Unix())))
	vals := st.Validators()
	currCommittee := &zondpbalpha.SyncCommittee{}
	for i := 0; i < 5; i++ {
		currCommittee.Pubkeys = append(currCommittee.Pubkeys, vals[i].PublicKey)
	}
	// add one public key twice - this is needed for one of the test cases
	currCommittee.Pubkeys = append(currCommittee.Pubkeys, vals[0].PublicKey)
	require.NoError(t, st.SetCurrentSyncCommittee(currCommittee))
	nextCommittee := &zondpbalpha.SyncCommittee{}
	for i := 5; i < 10; i++ {
		nextCommittee.Pubkeys = append(nextCommittee.Pubkeys, vals[i].PublicKey)
	}
	require.NoError(t, st.SetNextSyncCommittee(nextCommittee))

	mockChainService := &mockChain.ChainService{Genesis: genesisTime}
	s := &Server{
		Stater:                &testutil.MockStater{BeaconState: st},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		TimeFetcher:           mockChainService,
		HeadFetcher:           mockChainService,
		OptimisticModeFetcher: mockChainService,
	}

	t.Run("single validator", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, hexutil.Encode(vals[1].PublicKey), duty.Pubkey)
		assert.Equal(t, "1", duty.ValidatorIndex)
		require.Equal(t, 1, len(duty.ValidatorSyncCommitteeIndices))
		assert.Equal(t, "1", duty.ValidatorSyncCommitteeIndices[0])
	})
	t.Run("multiple validators", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"1\",\"2\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("invalid", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"foo\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
	})
	t.Run("validator without duty not returned", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"1\",\"10\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].ValidatorIndex)
	})
	t.Run("multiple indices for validator", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		duty := resp.Data[0]
		require.Equal(t, 2, len(duty.ValidatorSyncCommitteeIndices))
		assert.DeepEqual(t, []string{"0", "5"}, duty.ValidatorSyncCommitteeIndices)
	})
	t.Run("validator index out of bound", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString(fmt.Sprintf("[\"%d\"]", numVals))
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Invalid validator index", e.Message)
	})
	t.Run("next sync committee period", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"5\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, hexutil.Encode(vals[5].PublicKey), duty.Pubkey)
		assert.Equal(t, "5", duty.ValidatorIndex)
		require.Equal(t, 1, len(duty.ValidatorSyncCommitteeIndices))
		assert.Equal(t, "0", duty.ValidatorSyncCommitteeIndices[0])
	})
	t.Run("correct sync committee is fetched", func(t *testing.T) {
		// in this test we swap validators in the current and next sync committee inside the new state

		newSyncPeriodStartSlot := primitives.Slot(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * uint64(params.BeaconConfig().SlotsPerEpoch))
		newSyncPeriodSt, _ := util.DeterministicGenesisStateCapella(t, numVals)
		require.NoError(t, newSyncPeriodSt.SetSlot(newSyncPeriodStartSlot))
		require.NoError(t, newSyncPeriodSt.SetGenesisTime(uint64(genesisTime.Unix())))
		vals := newSyncPeriodSt.Validators()
		currCommittee := &zondpbalpha.SyncCommittee{}
		for i := 5; i < 10; i++ {
			currCommittee.Pubkeys = append(currCommittee.Pubkeys, vals[i].PublicKey)
		}
		require.NoError(t, newSyncPeriodSt.SetCurrentSyncCommittee(currCommittee))
		nextCommittee := &zondpbalpha.SyncCommittee{}
		for i := 0; i < 5; i++ {
			nextCommittee.Pubkeys = append(nextCommittee.Pubkeys, vals[i].PublicKey)
		}
		require.NoError(t, newSyncPeriodSt.SetNextSyncCommittee(nextCommittee))

		stateFetchFn := func(slot primitives.Slot) state.BeaconState {
			if slot < newSyncPeriodStartSlot {
				return st
			} else {
				return newSyncPeriodSt
			}
		}
		mockChainService := &mockChain.ChainService{Genesis: genesisTime, Slot: &newSyncPeriodStartSlot}
		s := &Server{
			Stater:                &testutil.MockStater{BeaconState: stateFetchFn(newSyncPeriodStartSlot)},
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			TimeFetcher:           mockChainService,
			HeadFetcher:           mockChainService,
			OptimisticModeFetcher: mockChainService,
		}

		var body bytes.Buffer
		_, err := body.WriteString("[\"8\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, hexutil.Encode(vals[8].PublicKey), duty.Pubkey)
		assert.Equal(t, "8", duty.ValidatorIndex)
		require.Equal(t, 1, len(duty.ValidatorSyncCommitteeIndices))
		assert.Equal(t, "3", duty.ValidatorSyncCommitteeIndices[0])
	})
	t.Run("epoch not at period start", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		duty := resp.Data[0]
		assert.Equal(t, hexutil.Encode(vals[1].PublicKey), duty.Pubkey)
		assert.Equal(t, "1", duty.ValidatorIndex)
		require.Equal(t, 1, len(duty.ValidatorSyncCommitteeIndices))
		assert.Equal(t, "1", duty.ValidatorSyncCommitteeIndices[0])
	})
	t.Run("epoch too far in the future", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"5\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": strconv.FormatUint(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod*2), 10)})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Epoch is too far in the future", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		ctx := context.Background()
		db := dbutil.SetupDB(t)
		require.NoError(t, db.SaveStateSummary(ctx, &zondpbalpha.StateSummary{Slot: 0, Root: []byte("root")}))
		require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &zondpbalpha.Checkpoint{Epoch: 0, Root: []byte("root")}))

		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		slot, err := slots.EpochStart(1)
		require.NoError(t, err)

		st2, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, st2.SetSlot(slot))

		mockChainService := &mockChain.ChainService{
			Genesis:    genesisTime,
			Optimistic: true,
			Slot:       &slot,
			FinalizedCheckPoint: &zondpbalpha.Checkpoint{
				Root:  root[:],
				Epoch: 1,
			},
			State: st2,
		}
		s := &Server{
			Stater:                &testutil.MockStater{BeaconState: st},
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			TimeFetcher:           mockChainService,
			HeadFetcher:           mockChainService,
			OptimisticModeFetcher: mockChainService,
			ChainInfoFetcher:      mockChainService,
			BeaconDB:              db,
		}

		var body bytes.Buffer
		_, err = body.WriteString("[\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetSyncCommitteeDutiesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("sync not ready", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		chainService := &mockChain.ChainService{State: st}
		s := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://www.example.com/eth/v1/validator/duties/sync/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetSyncCommitteeDuties(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusServiceUnavailable, e.Code)
	})
}

func TestPrepareBeaconProposer(t *testing.T) {
	tests := []struct {
		name    string
		request []*shared.FeeRecipient
		code    int
		wantErr string
	}{
		{
			name: "Happy Path",
			request: []*shared.FeeRecipient{{
				FeeRecipient:   "Zb698D697092822185bF0311052215d5B5e1F3934",
				ValidatorIndex: "1",
			},
			},
			code:    http.StatusOK,
			wantErr: "",
		},
		{
			name: "invalid fee recipient length",
			request: []*shared.FeeRecipient{{
				FeeRecipient:   "Zb698D697092822185bF0311052",
				ValidatorIndex: "1",
			},
			},
			code:    http.StatusBadRequest,
			wantErr: "Invalid Fee Recipient",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.request)
			require.NoError(t, err)
			var body bytes.Buffer
			_, err = body.WriteString(string(b))
			require.NoError(t, err)
			url := "http://example.com/eth/v1/validator/prepare_beacon_proposer"
			request := httptest.NewRequest(http.MethodPost, url, &body)
			writer := httptest.NewRecorder()
			db := dbutil.SetupDB(t)
			ctx := context.Background()
			server := &Server{
				BeaconDB: db,
			}
			server.PrepareBeaconProposer(writer, request)
			require.Equal(t, tt.code, writer.Code)
			if tt.wantErr != "" {
				require.Equal(t, strings.Contains(writer.Body.String(), tt.wantErr), true)
			} else {
				require.NoError(t, err)
				address, err := server.BeaconDB.FeeRecipientByValidatorID(ctx, 1)
				require.NoError(t, err)
				feebytes, err := hexutil.DecodeZ(tt.request[0].FeeRecipient)
				require.NoError(t, err)
				require.Equal(t, common.BytesToAddress(feebytes), address)
			}
		})
	}
}

func TestProposer_PrepareBeaconProposerOverlapping(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbutil.SetupDB(t)

	// New validator
	proposerServer := &Server{BeaconDB: db}
	req := []*shared.FeeRecipient{{
		FeeRecipient:   hexutil.EncodeZ(bytesutil.PadTo([]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)),
		ValidatorIndex: "1",
	}}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	var body bytes.Buffer
	_, err = body.WriteString(string(b))
	require.NoError(t, err)
	url := "http://example.com/eth/v1/validator/prepare_beacon_proposer"
	request := httptest.NewRequest(http.MethodPost, url, &body)
	writer := httptest.NewRecorder()

	proposerServer.PrepareBeaconProposer(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	require.LogsContain(t, hook, "Updated fee recipient addresses")

	// Same validator
	hook.Reset()
	_, err = body.WriteString(string(b))
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, url, &body)
	writer = httptest.NewRecorder()
	proposerServer.PrepareBeaconProposer(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	require.LogsDoNotContain(t, hook, "Updated fee recipient addresses")

	// Same validator with different fee recipient
	hook.Reset()
	req = []*shared.FeeRecipient{{
		FeeRecipient:   hexutil.EncodeZ(bytesutil.PadTo([]byte{0x01, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)),
		ValidatorIndex: "1",
	}}
	b, err = json.Marshal(req)
	require.NoError(t, err)
	_, err = body.WriteString(string(b))
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, url, &body)
	writer = httptest.NewRecorder()
	proposerServer.PrepareBeaconProposer(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	require.LogsContain(t, hook, "Updated fee recipient addresses")

	// More than one validator
	hook.Reset()
	req = []*shared.FeeRecipient{
		{
			FeeRecipient:   hexutil.EncodeZ(bytesutil.PadTo([]byte{0x01, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)),
			ValidatorIndex: "1",
		},
		{
			FeeRecipient:   hexutil.EncodeZ(bytesutil.PadTo([]byte{0x01, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)),
			ValidatorIndex: "2",
		},
	}
	b, err = json.Marshal(req)
	require.NoError(t, err)
	_, err = body.WriteString(string(b))
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, url, &body)
	writer = httptest.NewRecorder()
	proposerServer.PrepareBeaconProposer(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	require.LogsContain(t, hook, "Updated fee recipient addresses")

	// Same validators
	hook.Reset()
	b, err = json.Marshal(req)
	require.NoError(t, err)
	_, err = body.WriteString(string(b))
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPost, url, &body)
	writer = httptest.NewRecorder()
	proposerServer.PrepareBeaconProposer(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	require.LogsDoNotContain(t, hook, "Updated fee recipient addresses")
}

func BenchmarkServer_PrepareBeaconProposer(b *testing.B) {
	db := dbutil.SetupDB(b)
	proposerServer := &Server{BeaconDB: db}

	f := bytesutil.PadTo([]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)
	recipients := make([]*shared.FeeRecipient, 0)
	for i := 0; i < 10000; i++ {
		recipients = append(recipients, &shared.FeeRecipient{FeeRecipient: hexutil.EncodeZ(f), ValidatorIndex: fmt.Sprint(i)})
	}
	byt, err := json.Marshal(recipients)
	require.NoError(b, err)
	var body bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = body.WriteString(string(byt))
		require.NoError(b, err)
		url := "http://example.com/eth/v1/validator/prepare_beacon_proposer"
		request := httptest.NewRequest(http.MethodPost, url, &body)
		writer := httptest.NewRecorder()
		proposerServer.PrepareBeaconProposer(writer, request)
		if writer.Code != http.StatusOK {
			b.Fatal()
		}
	}
}

func TestGetLiveness(t *testing.T) {
	// Setup:
	// Epoch 0 - both validators not live
	// Epoch 1 - validator with index 1 is live
	// Epoch 2 - validator with index 0 is live
	oldSt, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, oldSt.AppendCurrentParticipationBits(0))
	require.NoError(t, oldSt.AppendCurrentParticipationBits(0))
	headSt, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, headSt.SetSlot(params.BeaconConfig().SlotsPerEpoch*2))
	require.NoError(t, headSt.AppendPreviousParticipationBits(0))
	require.NoError(t, headSt.AppendPreviousParticipationBits(1))
	require.NoError(t, headSt.AppendCurrentParticipationBits(1))
	require.NoError(t, headSt.AppendCurrentParticipationBits(0))

	s := &Server{
		HeadFetcher: &mockChain.ChainService{State: headSt},
		Stater: &testutil.MockStater{
			// We configure states for last slots of an epoch
			StatesBySlot: map[primitives.Slot]state.BeaconState{
				params.BeaconConfig().SlotsPerEpoch - 1:   oldSt,
				params.BeaconConfig().SlotsPerEpoch*3 - 1: headSt,
			},
		},
	}

	t.Run("old epoch", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetLivenessResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		data0 := resp.Data[0]
		data1 := resp.Data[1]
		assert.Equal(t, true, (data0.Index == "0" && !data0.IsLive) || (data0.Index == "1" && !data0.IsLive))
		assert.Equal(t, true, (data1.Index == "0" && !data1.IsLive) || (data1.Index == "1" && !data1.IsLive))
	})
	t.Run("previous epoch", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetLivenessResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		data0 := resp.Data[0]
		data1 := resp.Data[1]
		assert.Equal(t, true, (data0.Index == "0" && !data0.IsLive) || (data0.Index == "1" && data0.IsLive))
		assert.Equal(t, true, (data1.Index == "0" && !data1.IsLive) || (data1.Index == "1" && data1.IsLive))
	})
	t.Run("current epoch", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "2"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetLivenessResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		data0 := resp.Data[0]
		data1 := resp.Data[1]
		assert.Equal(t, true, (data0.Index == "0" && data0.IsLive) || (data0.Index == "1" && !data0.IsLive))
		assert.Equal(t, true, (data1.Index == "0" && data1.IsLive) || (data1.Index == "1" && !data1.IsLive))
	})
	t.Run("future epoch", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "3"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		require.StringContains(t, "Requested epoch cannot be in the future", e.Message)
	})
	t.Run("no epoch provided", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Epoch is required"))
	})
	t.Run("invalid epoch provided", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "foo"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.Equal(t, true, strings.Contains(e.Message, "Epoch is invalid"))
	})
	t.Run("no body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", nil)
		request = mux.SetURLVars(request, map[string]string{"epoch": "3"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("empty", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "3"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "No data submitted", e.Message)
	})
	t.Run("unknown validator index", func(t *testing.T) {
		var body bytes.Buffer
		_, err := body.WriteString("[\"0\",\"1\",\"2\"]")
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/liveness/{epoch}", &body)
		request = mux.SetURLVars(request, map[string]string{"epoch": "0"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLiveness(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		require.StringContains(t, "Validator index 2 is invalid", e.Message)
	})
}

var (
	singleContribution = `[
  {
    "message": {
      "aggregator_index": "1",
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
      "contribution": {
        "slot": "1",
        "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
        "subcommittee_index": "1",
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
      }
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]`
	multipleContributions = `[
  {
    "message": {
      "aggregator_index": "1",
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
      "contribution": {
        "slot": "1",
        "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
        "subcommittee_index": "1",
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
      }
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  },
  {
    "message": {
      "aggregator_index": "1",
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
      "contribution": {
        "slot": "1",
        "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
        "subcommittee_index": "1",
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
      }
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]`
	// aggregator_index is invalid
	invalidContribution = `[
  {
    "message": {
      "aggregator_index": "foo",
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
      "contribution": {
        "slot": "1",
        "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
        "subcommittee_index": "1",
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
      }
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]`
	singleAggregate = `[
  {
    "message": {
      "aggregator_index": "1",
      "aggregate": {
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
        "data": {
          "slot": "1",
          "index": "1",
          "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
          "source": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          },
          "target": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          }
        }
      },
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]`
	multipleAggregates = `[
  {
    "message": {
      "aggregator_index": "1",
      "aggregate": {
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
        "data": {
          "slot": "1",
          "index": "1",
          "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
          "source": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          },
          "target": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          }
        }
      },
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  },
{
    "message": {
      "aggregator_index": "1",
      "aggregate": {
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
        "data": {
          "slot": "1",
          "index": "1",
          "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
          "source": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          },
          "target": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          }
        }
      },
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]
`
	// aggregator_index is invalid
	invalidAggregate = `[
  {
    "message": {
      "aggregator_index": "foo",
      "aggregate": {
        "aggregation_bits": "0x01",
        "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c",
        "data": {
          "slot": "1",
          "index": "1",
          "beacon_block_root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
          "source": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          },
          "target": {
            "epoch": "1",
            "root": "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
          }
        }
      },
      "selection_proof": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }
]`
	singleSyncCommitteeSubscription = `[
  {
    "validator_index": "1",
    "sync_committee_indices": [
      "0",
      "2"
    ],
    "until_epoch": "1"
  }
]`
	singleSyncCommitteeSubscription2 = `[
  {
    "validator_index": "0",
    "sync_committee_indices": [
      "0",
      "2"
    ],
    "until_epoch": "0"
  }
]`
	singleSyncCommitteeSubscription3 = fmt.Sprintf(`[
  {
    "validator_index": "0",
    "sync_committee_indices": [
      "0",
      "2"
    ],
    "until_epoch": "%d"
  }
]`, 2*params.BeaconConfig().EpochsPerSyncCommitteePeriod)
	singleSyncCommitteeSubscription4 = fmt.Sprintf(`[
  {
    "validator_index": "0",
    "sync_committee_indices": [
      "0",
      "2"
    ],
    "until_epoch": "%d"
  }
]`, 2*params.BeaconConfig().EpochsPerSyncCommitteePeriod+1)
	multipleSyncCommitteeSubscription = `[
  {
    "validator_index": "0",
    "sync_committee_indices": [
      "0"
    ],
    "until_epoch": "1"
  },
  {
    "validator_index": "1",
    "sync_committee_indices": [
      "2"
    ],
    "until_epoch": "1"
  }
]`
	// validator_index is invalid
	invalidSyncCommitteeSubscription = `[
  {
    "validator_index": "foo",
    "sync_committee_indices": [
      "0",
      "2"
    ],
    "until_epoch": "1"
  }
]`
	singleBeaconCommitteeContribution = `[
  {
    "validator_index": "1",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": false
  }
]`
	singleBeaconCommitteeContribution2 = `[
  {
    "validator_index": "1",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": true
  }
]`
	multipleBeaconCommitteeContribution = `[
  {
    "validator_index": "1",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": false
  },
  {
    "validator_index": "2",
    "committee_index": "0",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": false
  }
]`
	multipleBeaconCommitteeContribution2 = `[
  {
    "validator_index": "1",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": true
  },
  {
    "validator_index": "2",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": false
  }
]`
	// validator_index is invalid
	invalidBeaconCommitteeContribution = `[
  {
    "validator_index": "foo",
    "committee_index": "1",
    "committees_at_slot": "2",
    "slot": "1",
    "is_aggregator": false
  }
]`
	registrations = `[{
    "message": {
      "fee_recipient": "Zabcf8e0d4e9587369b2301d0790347320302cc09",
      "gas_limit": "1",
      "timestamp": "1",
      "pubkey": "0xe0a586bb51db522c31abcbce14e6cbf6a5bbc7b3331cdb76378ca1b98acff048c11099c2713f229c349c430a6aa5623fab8d39ec266e0e7d81543fc2e4b905ec7fba75b9ab3aae53e18e2a018297ebe4bb2d0a22bd13b60b938461d922ec81dfe152224c51abcccd4105799ee2b70b53cf2401a3c01664c20ab368c4c3ccc764be5063488750f79480adcac444e274fb46500aeb2449d2a81e44c3528c70554a9ecd5b25b39550d469a43f5ec2afce668aa6598aa1c5618e569bdf08ec700a21950d6d2df3337ff196b6fcb53de94e7e127dbd7edf9c5df70c41c715b48cf4e5ab5d0e1bc4d9ead578150f98244ea47dba29af25b12a72054618d0341ebbae8e5c61cf6583c0151fdcf1323ee3cd65f8f739dc621f2aa77f8dfe36a7cef15162972c25a193bd306918deb8d6395367586ee6a534340c07caf6496dc393a0189cf81325499132a012a2b8a6152be3d3d010aadba896af83d9d447741a66100f72da46c9282a8a9af5bfec0d84d88882ce0a090147dbcef2f100f8744094a8e3712c26d875996f56a92fd99a39537197f0bbe58bb706061426e62406a300626f64b7dd813c756c159cea82a6cf82b9890be40284720b9aa9c6f1a3a78bfb8607b438ec3665225fe21370770cfdbbc20ef6525362d413ab23e85d5f6ef38a43b44874828137ab977dd9a145913ecffe2700a225042b766158d26288434511014928efdb857df4217430e18bd6c370c8327b4451611c66a118193f1155ffeef32d9b26b02d04cd083964f53b59b5ffd02789be6e8aeae4f615afb39e53f5cfbf3d9ec23640fea711fc6751abe9b3606959ac12aeb827a76a515f27d0e0f1e003e00a91a1d20b97ecde53202d6f9d61a1d4b0bd7d4f0622c2a90d67ba40f59a450191aa340bcc7b3b3107830ce1fb791a97930fd68c6b9ea0848c0591edb0a6302e0984d7f096ccb980803bbeaae3550d8996a001ecba956d3c2bb20eaa33094071e639983693f64809e449c29b59bc0b4f1530ec273f366db337bc64d95a9e26cc21ed0685cb2c606b994505bdd6237dfdb414df7eee7544f34cdf5f0e6ca1e5280b493446cb883413e26e06a00354bced7a5fd410fd92ffc39443d9e8f208aec8d81d958c060203bbe75db0cb2b982524b5e91135d4ef671ebe6c55c24bdb00d89b78c7d8fed674d1fac6d6d61bb671a996d3efe27a254e40967cb60c3c7ac5814ca5e5768f268c7002ba200da9200fc5498d07833c4b25a111d35f64cd26b108a897616d4324984e0833937344b904b964d5f292eeba6075987b5cc092bd40697ef9b2ea95cbc1eed5bf3f6337145351e98291853c3bf75eb1a533817cab5dc8d87abf034696e8dc9ad20089d79086b8608cb07101b62ae744beba3cc71e75701d46c35f317ecd1f3bb6d6078bd8cf25a55bbd200d7bfb5c9e3e2167a6abe8a6636ca82bdd63c3007b39d9a57b9f258ee4bb94325b20744087ba3a2bbda513ed067b003a0d6a4197bae5776cb25899911f92e3cdd779931e98cb11846dac49480af3c2fc3596825ccbb7a7dfa3e714d8fc809acc57577dd448e477bff03a907f8410f2ae12a9ab4a3d7738315c07f42e5af416560aafa035ae4d4b72d5e59a45dcd4c91000cd8cef454c7a157276cb2610d2d08a7bc90550c85e317fdb2d83ee26f49198dd035bbe39d6eedfbc91a1cffb5682f0410204c281f3cb8d702258c77214d77e92f1e5f4db2a6be18911c5f3950a3228d1850722f4ce0a5d56a8acf2e0311290e1334a2bfbf1251d6cb46f2a028aaa7be144f38fdb222e8a2d6320f98796731847b2449774c025a452c72dfbb9f05959c88ed86256f5fcea5458c3e22340d8ad3c3ff548f03346c55f74d6ab3aff1311a302bb8c5cd55b44528fd08ecab030c1e47385ed27de5819fd798ada8858462de7fd55aa7239e03079d976669e52ff22bf9ab6fa4860064dd5033ca6ae1fec5c628e5bc2b190ae5483514d841a25b04d127d9c536e32f3bada7b46cbb14b5718c88ed826a8c19d1fd43a7f7ff6860a88adf9fcf1415eb2c56e12a7dc6a73e24bd7cbcc7fa39ce7358f20736adc11842e72a5107bcbe78f56bb56fe403d51ea531d7d4f2681fd05f5326d7e5dd7e889a3380f9dfe8124d8f258d6f9ba6f0f6467e787d996da6310196a70f551e64d1a9dc51fe907227f43a1fb54a572db183edb726375a7a1096daeb8d7b069bf8886d282dabdb7e9101fadcdfb23c57be75a193cc3459401d14836d250b197e6ae0e4818b2bea75db388bf36f311eff18ac14b9f0fe1a354d8d397439fd202d61d545f430676eb16c6ecc4c3f583fa8767d65cdc4f3155af47629cf1b0b833a12391b02b1781f1c31cb6b05160241b1e02c5889db631ad2fa905d608b2831b45529dd7550d5ff91d4b7ae23533b1f6875b38be0f26f4479cb75579d8612ff2cfec981344598c76054584f6350d296c2436e2d43f184556d4e6208483e010ba8bcdf413d659fab3353bb8f53f085dfe24910f28b82ae382047383e81922f2d05b13d073b3fbb8042c9c1dd6a073109afffac32117a6b4162387949a9c2b21661eb321a340978b4c43dcb8ce264d6e30751c1e91551f4c2efec349bf0f083db63f3bbbbc83be7eb044b17a7fbfdeedbdd80e76580a5082d7534cea34620997ee593fc0c725a9cc41f192cdcb85d2021f2dafea48f14f63d01329845c0533210075ac3d1b674a5535d37c5c5acbd8fdee0ef9d3dc66b9fdec661f3ed53d1c70c825937716af2d44510d07876b3d52c063e7ddca41faee15d3b81940dd50d41ad5791b4b37f44cecc11db9ce58c7555491ee822e8ff1d8b0dac3eb409f8b827561ea6b7d88af82892a53ff2d239c76a8a1a717b101c7b9d7db85a84a508276b8d1ba972d31089cce5dba3ed722ead0849d336b1002f41f1b1b93d2a7e56e5c222d21327d872534aae80e8f7020c4fda6fd4765bd94df4aa38c51924b356412ae0ceff6adfdad9b9c793ee6aec73a902f658ffd6af25abf374368e38a8b9e91b34d2eeca566eb39ffa67978077870b21279afc7760f38639ad6fa152af670f25de919fcecc16755bafff466a0b8d9910bf84bc5917a33ed76fd62c47a9a2ca68055668a13f11616b7f95cda26c2b09bbe8c83609af99ec41470bda5b12524a849950caf6fb96d908dabca97187858c83a54cb2dee7fcabbc0fea8d3ca1b860d1d7b5eb1dc2a687330d2fb237f55d97fbab4694e4037355548f1c20122da77eb0b7b90205989bb9ef52f76f88770eabf56f5d9ccaf572b3eadb7c9810e93b675e7e9ea26f8d8749fcb23c63993d62406db2a53996dc053e698e70360492e2c467e1baf2d76a9dc74f23c3be3d27685c5fd07a30d2aab3f2cd7fde563e29a3434ee6a51f795a5e114a3d6259732362126da789d82ee54dae91c3e2c060a4f79943068cb6a3ee5692587a67816aa5a9c5ff3805173c72a5ad2b0ebd8588253bae50da117d938901f8ffbf725ced16a76f9f53d782ebe1f0d6f6dfdaa4fe8f93ec6246b66e561c740fc7eaf6c771659e90f545b9e89221fc9450543424f0a14ad7484253251f658e56cf1cb161b4cee63c6c5b96cf8c06e6aa524c8209205de7fdbf1e233135755ed6300ed4c096764fe4dd4855f421d272cd63150db47bc6f47bf624798"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  },{
    "message": {
      "fee_recipient": "Zabcf8e0d4e9587369b2301d0790347320302cc09",
      "gas_limit": "1",
      "timestamp": "1",
      "pubkey": "0xe0a586bb51db522c31abcbce14e6cbf6a5bbc7b3331cdb76378ca1b98acff048c11099c2713f229c349c430a6aa5623fab8d39ec266e0e7d81543fc2e4b905ec7fba75b9ab3aae53e18e2a018297ebe4bb2d0a22bd13b60b938461d922ec81dfe152224c51abcccd4105799ee2b70b53cf2401a3c01664c20ab368c4c3ccc764be5063488750f79480adcac444e274fb46500aeb2449d2a81e44c3528c70554a9ecd5b25b39550d469a43f5ec2afce668aa6598aa1c5618e569bdf08ec700a21950d6d2df3337ff196b6fcb53de94e7e127dbd7edf9c5df70c41c715b48cf4e5ab5d0e1bc4d9ead578150f98244ea47dba29af25b12a72054618d0341ebbae8e5c61cf6583c0151fdcf1323ee3cd65f8f739dc621f2aa77f8dfe36a7cef15162972c25a193bd306918deb8d6395367586ee6a534340c07caf6496dc393a0189cf81325499132a012a2b8a6152be3d3d010aadba896af83d9d447741a66100f72da46c9282a8a9af5bfec0d84d88882ce0a090147dbcef2f100f8744094a8e3712c26d875996f56a92fd99a39537197f0bbe58bb706061426e62406a300626f64b7dd813c756c159cea82a6cf82b9890be40284720b9aa9c6f1a3a78bfb8607b438ec3665225fe21370770cfdbbc20ef6525362d413ab23e85d5f6ef38a43b44874828137ab977dd9a145913ecffe2700a225042b766158d26288434511014928efdb857df4217430e18bd6c370c8327b4451611c66a118193f1155ffeef32d9b26b02d04cd083964f53b59b5ffd02789be6e8aeae4f615afb39e53f5cfbf3d9ec23640fea711fc6751abe9b3606959ac12aeb827a76a515f27d0e0f1e003e00a91a1d20b97ecde53202d6f9d61a1d4b0bd7d4f0622c2a90d67ba40f59a450191aa340bcc7b3b3107830ce1fb791a97930fd68c6b9ea0848c0591edb0a6302e0984d7f096ccb980803bbeaae3550d8996a001ecba956d3c2bb20eaa33094071e639983693f64809e449c29b59bc0b4f1530ec273f366db337bc64d95a9e26cc21ed0685cb2c606b994505bdd6237dfdb414df7eee7544f34cdf5f0e6ca1e5280b493446cb883413e26e06a00354bced7a5fd410fd92ffc39443d9e8f208aec8d81d958c060203bbe75db0cb2b982524b5e91135d4ef671ebe6c55c24bdb00d89b78c7d8fed674d1fac6d6d61bb671a996d3efe27a254e40967cb60c3c7ac5814ca5e5768f268c7002ba200da9200fc5498d07833c4b25a111d35f64cd26b108a897616d4324984e0833937344b904b964d5f292eeba6075987b5cc092bd40697ef9b2ea95cbc1eed5bf3f6337145351e98291853c3bf75eb1a533817cab5dc8d87abf034696e8dc9ad20089d79086b8608cb07101b62ae744beba3cc71e75701d46c35f317ecd1f3bb6d6078bd8cf25a55bbd200d7bfb5c9e3e2167a6abe8a6636ca82bdd63c3007b39d9a57b9f258ee4bb94325b20744087ba3a2bbda513ed067b003a0d6a4197bae5776cb25899911f92e3cdd779931e98cb11846dac49480af3c2fc3596825ccbb7a7dfa3e714d8fc809acc57577dd448e477bff03a907f8410f2ae12a9ab4a3d7738315c07f42e5af416560aafa035ae4d4b72d5e59a45dcd4c91000cd8cef454c7a157276cb2610d2d08a7bc90550c85e317fdb2d83ee26f49198dd035bbe39d6eedfbc91a1cffb5682f0410204c281f3cb8d702258c77214d77e92f1e5f4db2a6be18911c5f3950a3228d1850722f4ce0a5d56a8acf2e0311290e1334a2bfbf1251d6cb46f2a028aaa7be144f38fdb222e8a2d6320f98796731847b2449774c025a452c72dfbb9f05959c88ed86256f5fcea5458c3e22340d8ad3c3ff548f03346c55f74d6ab3aff1311a302bb8c5cd55b44528fd08ecab030c1e47385ed27de5819fd798ada8858462de7fd55aa7239e03079d976669e52ff22bf9ab6fa4860064dd5033ca6ae1fec5c628e5bc2b190ae5483514d841a25b04d127d9c536e32f3bada7b46cbb14b5718c88ed826a8c19d1fd43a7f7ff6860a88adf9fcf1415eb2c56e12a7dc6a73e24bd7cbcc7fa39ce7358f20736adc11842e72a5107bcbe78f56bb56fe403d51ea531d7d4f2681fd05f5326d7e5dd7e889a3380f9dfe8124d8f258d6f9ba6f0f6467e787d996da6310196a70f551e64d1a9dc51fe907227f43a1fb54a572db183edb726375a7a1096daeb8d7b069bf8886d282dabdb7e9101fadcdfb23c57be75a193cc3459401d14836d250b197e6ae0e4818b2bea75db388bf36f311eff18ac14b9f0fe1a354d8d397439fd202d61d545f430676eb16c6ecc4c3f583fa8767d65cdc4f3155af47629cf1b0b833a12391b02b1781f1c31cb6b05160241b1e02c5889db631ad2fa905d608b2831b45529dd7550d5ff91d4b7ae23533b1f6875b38be0f26f4479cb75579d8612ff2cfec981344598c76054584f6350d296c2436e2d43f184556d4e6208483e010ba8bcdf413d659fab3353bb8f53f085dfe24910f28b82ae382047383e81922f2d05b13d073b3fbb8042c9c1dd6a073109afffac32117a6b4162387949a9c2b21661eb321a340978b4c43dcb8ce264d6e30751c1e91551f4c2efec349bf0f083db63f3bbbbc83be7eb044b17a7fbfdeedbdd80e76580a5082d7534cea34620997ee593fc0c725a9cc41f192cdcb85d2021f2dafea48f14f63d01329845c0533210075ac3d1b674a5535d37c5c5acbd8fdee0ef9d3dc66b9fdec661f3ed53d1c70c825937716af2d44510d07876b3d52c063e7ddca41faee15d3b81940dd50d41ad5791b4b37f44cecc11db9ce58c7555491ee822e8ff1d8b0dac3eb409f8b827561ea6b7d88af82892a53ff2d239c76a8a1a717b101c7b9d7db85a84a508276b8d1ba972d31089cce5dba3ed722ead0849d336b1002f41f1b1b93d2a7e56e5c222d21327d872534aae80e8f7020c4fda6fd4765bd94df4aa38c51924b356412ae0ceff6adfdad9b9c793ee6aec73a902f658ffd6af25abf374368e38a8b9e91b34d2eeca566eb39ffa67978077870b21279afc7760f38639ad6fa152af670f25de919fcecc16755bafff466a0b8d9910bf84bc5917a33ed76fd62c47a9a2ca68055668a13f11616b7f95cda26c2b09bbe8c83609af99ec41470bda5b12524a849950caf6fb96d908dabca97187858c83a54cb2dee7fcabbc0fea8d3ca1b860d1d7b5eb1dc2a687330d2fb237f55d97fbab4694e4037355548f1c20122da77eb0b7b90205989bb9ef52f76f88770eabf56f5d9ccaf572b3eadb7c9810e93b675e7e9ea26f8d8749fcb23c63993d62406db2a53996dc053e698e70360492e2c467e1baf2d76a9dc74f23c3be3d27685c5fd07a30d2aab3f2cd7fde563e29a3434ee6a51f795a5e114a3d6259732362126da789d82ee54dae91c3e2c060a4f79943068cb6a3ee5692587a67816aa5a9c5ff3805173c72a5ad2b0ebd8588253bae50da117d938901f8ffbf725ced16a76f9f53d782ebe1f0d6f6dfdaa4fe8f93ec6246b66e561c740fc7eaf6c771659e90f545b9e89221fc9450543424f0a14ad7484253251f658e56cf1cb161b4cee63c6c5b96cf8c06e6aa524c8209205de7fdbf1e233135755ed6300ed4c096764fe4dd4855f421d272cd63150db47bc6f47bf624798"
    },
    "signature": "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"
  }]`
)
