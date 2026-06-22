package beacon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/theQRL/go-qrl/common/hexutil"
	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	http2 "github.com/theQRL/qrysm/network/http"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestGetValidators(t *testing.T) {
	var st state.BeaconState
	st, _ = util.DeterministicGenesisStateZond(t, 8192)

	t.Run("get all", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 8192, len(resp.Data))
		val := resp.Data[0]
		assert.Equal(t, "0", val.Index)
		assert.Equal(t, "40000000000000", val.Balance)
		assert.Equal(t, "active_ongoing", val.Status)
		require.NotNil(t, val.Validator)
		assert.Equal(t, "0x2e3bcd95f5e30d0b7c5577b66c6373c567a6151ea39a75e477fb9b979276efac8952fcab3b1e671c1903063c08de679ae4fdda1e3f2cf675364a0f9fa5cebb7a6d69dfd6fd5b060dce52f19c3d95337ed388b53ea8d9340860cdfe449187d617e072979f84d7c9bc1a7169706b6fbe01e51c6c8117be3d61e79c046827d095a6f88d4c452da251910d34c7e77e773d667db344535b71998d33920d21d03f16c8b60d27448d24dbe239771bb312be22ef881e919a8bb2cf63108d6122f1ccdd3dce3ccc39a4bf06bfc8acdfeac9261cb061b64e1210dbd70d1da94145fb46e114fd61111c7e28f96a8023b4b3d2b705e347d9d5f629e0c64a533eaf0a84095ccb609196b768d1212db3d957fbf5df67fc8a14c72fba9659ae677647c24dd9d121094ac1cac39f14fbbe5e8624808f004e970b3530c5118cfbb1f041308eba8c8a5bec6ac20805790eb793c21eed3908c0ee74075ebe0f8367d46341f548326a9dfe96dc237a7e103010c5810472d071dc86a3f12936e8f3181eb8de498719acafe20cd59f8291b0313fa70f9843fa65a49712e92516accbb9d2c30c341b5341d618db57299937b7bbd7d51ee9b5e1329cfd725988ad4d8f01389497bfcfe904bbebfd69e9adaf1afa15136f2fe355d4e54aa4c21a4c41bf1a9172535504058d7aaceb9776f16e5c209204b755de6a978cc9952d4bf45b24718cab949e198fef31bc14a09f8474ee0037b758bfb952f5bf0a5f2231c02dcded4c85600a447b80ffd39d6a07c9dedd7efdd45b9c855d9a9b6a88f1f14f405a7608c1f64cd815df316a3ad8442b75df79aade1a9156eb917ad18251406a277cf38f9870a7902b4422931d73406f87e0802e96965773935d2639a649863198d83946425c40a84413d2b1bca109573dc6298584b8c589373be2ca7732478b220b1a214182f96a26c5a1b45dda66d2385346301d167d30202b4fa6167cfb87ecf0377d33a7cac6f38b29ff384481c4531e47a94197334e8ff9f77eb868813f9d5ba5249d9006a2317397cafdd206375d9654eceff93115a530e84ae55a7a25d02d44d737f42da58486c7dc3e92f3312ca80dfe2a9e87a52c6cfd852fb23767a9e551455ce7f2629855cc28c56e83cd6f2e1d4e3f05058b6a748fcdce270decc01149d96a9ab1f4f8ac1c72742f4fb0f602144e018697134d9ed2a5771bba1a8702ce94fc1732299080616348bb6eeaf7ed985e881f0a2a407b22e3a8426d4a4e2ceb257d4e635cb12c1706edc0fc77043652f3ccf3afa7d2b97c3b60bd0078d7220de39fd1e822e37c6827b870d1ce431015b03b1c8e2e88283bd8d7c6ca88673f7141d88c702d4f2bb35aa8f953249f429d7f80a67ed4352d5b2b79694f545f89d23e7e4a96c98647236c55bfe2d02f1fcfa254f9089baef4a5c8b9ecf5936cd6102500d67993ed92143dfca9c60caf49b25096c79bb9661839032507f99e8770702fa74babffe16da6b4c66fdf47e9fa03321ec973582dc4492c3025254037f55bfd47bed448105445ff7d9f0b2fbb84d798151c6d1255982dceda6abe534c1a487b9f0a2669faddce68840651c57daa54664e2dc11084867d98687a17758ddacbf0b2088e46f1c87deda65a904020c187e26fb22053aaa4daea57caaf9df6119909e727447d483492c961f72918e4dd8a7e543aa490b39c1f45e05b22f3e9023bbcadf0f91b3fdab1ac16aae2d6f13729219d5ae70b2ba8b552f23a25e56bad561d06c3c33a6578a966b9a9e8971c6008ad5791cd87ab2da27690b3f2868191a65abb6add5f72613a6ba333b1cbdea995cad79007eacc931061cb6c107b55973a87d15bc9c95d2d9e4dec237ddf91672251a05853dc97fe3ed27196814cb6e84f1e28954693475d382b6ec7ddb594613325d2c82a7dd9bb6a429ee0037cd8af2f129d871eef4da33966edecd011f0d271e527fb694430833bd7acc3b2944599ba3e13aae3a1213bf265dccd53123b8b285d9158d072699fc8c68d461550abe4794ff7397d1fc1adde7502be6e93399cc5ca0963d38be6923b033c6873b9ce8d05c0de899fb116ffd20822d8d956d1dcdc823f5b3682cb12491d462c1d98600f2b48ad02ba7090e2833c8b8f374229074d21f13b3d9c0923f78624e8fe2caa8ec57d7b321494d796f742e93c9e049cb416b05a17eeba52799113cb42274ea07fa7fb8405f5ea726eebcf15da0173c344f7f85da412589dd30c9a9b858e1cb3d36fdf3bbe988ff83ecbcaae06a40a7cde8225b5e02edd615a0c89126bead1380a4c8843ad313cb94b040dbda8a354dd11e2725584d500fb61adb7f785f253e153998838e5b87374f1c759e48da79f1d3e44c3c0c0ac3b13473abad9cfdf60f5b572387c7bebaa4a419d8a0cd26e73ec58ea92f80de3f9aca31f1faedf94d82b2b35f2520d4f6167c6e886f84620d79d7d8859f8562bff408f7fb3bea11c3ffdd8074748e10b22ed1bb5955daf6122c8daeafa5541364845f149c67ff83a0f8517fc6727b4c997d637c3c46bf80c76e9e8858ea041cbfee49fc135fe8ef082fab0dcb9c795f81add8e0d7386bb2cee496948b9c8bd22d3230d400b0beb119297097a73e41760df8ae3f22ab40d0402238833c5e12d90fecb1ce9c88c68f281baaef15cdadcdad4670ab0dec84b14adb9c0325fc63a05bc66fbff3bdf45a114f42c232ff53d8bfb3a474ce5c4f71841ee198336d4eeec82d52df5c4098f25bcc88f894a091cfc63203d3d50b5d456143467018da3795ee9d84508027a503550a813be30a3151bc1ca3768307d381e9184df8a3d00b20cedde0d82a728e2e714d1461deefa6acc067397ce836500289b57ac535aa19317dcf27c20460803ba777e55b8e29b9a678b67c6d6ef7abb82fdf976ef56eff535664dc28ec94b8b78193843be5ff58c2180b9559fb14749e9b7139d83fe2cf9fb375f134dc51e79831d932aefac40aeb83e006679952f95820882de19027172822cd6365be331fc7a54bfea837277410c0f81362518967dbafab318c820212ec5515b4e7354d15815c080a52f31553a5e9081bfc38ce19ef227fde8e05d042146b2dfe0bd4cfcf927b73cf3472ea8718d7d4c62753353ce24e64f7cb330257498a44260d92e64968e3d89a55c33a93dbe1df6220abbdf6f675febc8bba04e3145fb61ad6f766625535826681ae969635b55f12220339d5fb644dbd9ba8b00d0a4c28b58ccb7df5eec10b93472ed0fe31f1d22be4848ef8b0e70c636a19ff2770fe5e7ee691fbc44820b71cc826a5a4d3995015a26f31ce0d0e2512d29705ff15a1faf7d88c45c7d67c10f5d38672ac69c2442cdb602de3f8553b077c07e39a3fa09a4588667b766133b402da1a77bcaf6eb2df58cd9b445e9eb1537654aff9e8c882e2a1a3e5ed46efb782515c0500056c422994cf8f18edd8c4422944a7fe16c1a59c2e098cdd7afc965af691f582142470953c0fc2e3a9ff2b8c87e483d1c811ce3a92a20341e7c1ead94448cbd49121d96258c657add51d9bfbf10b28710ec4d263da0e1d9b3ae8f09c462b9868e08097be87853408245ebcb6ca11fd2a3a088a50b5583f6938268ac37bb295edc096048eb5631b6a6b40dff8cb7fc21b76839aba5b869689fb6bf0cc19957962ac2d494eb03", val.Validator.Pubkey)
		assert.Equal(t, "0xd1f7b1b9069d4f8c7e623344ad8bfdaadaee96abf9be6c6fe344059656375e68586496de6e881b8087a991184d7e1d88d0a4f790de63ad078972b25bdbe2224f", val.Validator.WithdrawalCredentials)
		assert.Equal(t, "40000000000000", val.Validator.EffectiveBalance)
		assert.Equal(t, false, val.Validator.Slashed)
		assert.Equal(t, "0", val.Validator.ActivationEligibilityEpoch)
		assert.Equal(t, "0", val.Validator.ActivationEpoch)
		assert.Equal(t, "18446744073709551615", val.Validator.ExitEpoch)
		assert.Equal(t, "18446744073709551615", val.Validator.WithdrawableEpoch)
	})
	t.Run("get by index", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=15&id=26",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "15", resp.Data[0].Index)
		assert.Equal(t, "26", resp.Data[1].Index)
	})
	t.Run("get by pubkey", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubkey1 := st.PubkeyAtIndex(primitives.ValidatorIndex(20))
		pubkey2 := st.PubkeyAtIndex(primitives.ValidatorIndex(66))
		hexPubkey1 := hexutil.Encode(pubkey1[:])
		hexPubkey2 := hexutil.Encode(pubkey2[:])
		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=%s&id=%s", hexPubkey1, hexPubkey2),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "20", resp.Data[0].Index)
		assert.Equal(t, "66", resp.Data[1].Index)
	})
	t.Run("get by both index and pubkey", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(20))
		hexPubkey := hexutil.Encode(pubkey[:])
		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=%s&id=60", hexPubkey),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "20", resp.Data[0].Index)
		assert.Equal(t, "60", resp.Data[1].Index)
	})
	t.Run("state ID required", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
	t.Run("unknown pubkey is ignored", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(1))
		hexPubkey := hexutil.Encode(pubkey[:])
		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=%s&id=%s", hexPubkey, hexutil.Encode([]byte(strings.Repeat("x", fieldparams.MLDSA87PubkeyLength)))),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
	})
	t.Run("unknown index is ignored", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=1&id=99999", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		chainService := &chainMock.ChainService{Optimistic: true}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestListValidators_FilterByStatus(t *testing.T) {
	var st state.BeaconState
	st, _ = util.DeterministicGenesisStateZond(t, 8192)

	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	validators := []*qrysmpb.Validator{
		// Pending initialized.
		{
			ActivationEpoch:            farFutureEpoch,
			ActivationEligibilityEpoch: farFutureEpoch,
		},
		// Pending queued.
		{
			ActivationEpoch:            10,
			ActivationEligibilityEpoch: 4,
		},
		// Active ongoing.
		{
			ActivationEpoch: 0,
			ExitEpoch:       farFutureEpoch,
		},
		// Active slashed.
		{
			ActivationEpoch: 0,
			ExitEpoch:       30,
			Slashed:         true,
		},
		// Active exiting.
		{
			ActivationEpoch: 3,
			ExitEpoch:       30,
			Slashed:         false,
		},
		// Exited slashed (at epoch 35).
		{
			ActivationEpoch:   3,
			ExitEpoch:         30,
			WithdrawableEpoch: 40,
			Slashed:           true,
		},
		// Exited unslashed (at epoch 35).
		{
			ActivationEpoch:   3,
			ExitEpoch:         30,
			WithdrawableEpoch: 40,
			Slashed:           false,
		},
		// Withdrawable (at epoch 45).
		{
			ActivationEpoch:   3,
			ExitEpoch:         30,
			WithdrawableEpoch: 40,
			EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
			Slashed:           false,
		},
		// Withdrawal done (at epoch 45).
		{
			ActivationEpoch:   3,
			ExitEpoch:         30,
			WithdrawableEpoch: 40,
			EffectiveBalance:  0,
			Slashed:           false,
		},
	}
	for _, val := range validators {
		require.NoError(t, st.AppendValidator(val))
		require.NoError(t, st.AppendBalance(params.BeaconConfig().MaxEffectiveBalance))
	}

	t.Run("active", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &lookup.BeaconDbStater{
				ChainInfoFetcher: &chainMock.ChainService{State: st},
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators?status=active", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 8192+2, len(resp.Data))
		for _, vc := range resp.Data {
			assert.Equal(
				t,
				true,
				vc.Status == "active_ongoing" ||
					vc.Status == "active_exiting" ||
					vc.Status == "active_slashed",
			)
		}
	})
	t.Run("active_ongoing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &lookup.BeaconDbStater{
				ChainInfoFetcher: &chainMock.ChainService{State: st},
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators?status=active_ongoing", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 8192+1, len(resp.Data))
		for _, vc := range resp.Data {
			require.Equal(
				t,
				true,
				vc.Status == "active_ongoing",
			)
		}
	})
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*35))
	t.Run("exited", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &lookup.BeaconDbStater{
				ChainInfoFetcher: &chainMock.ChainService{State: st},
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators?status=exited", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 4, len(resp.Data))
		for _, vc := range resp.Data {
			require.Equal(
				t,
				true,
				vc.Status == "exited_unslashed" || vc.Status == "exited_slashed",
			)
		}
	})
	t.Run("pending_initialized and exited_unslashed", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &lookup.BeaconDbStater{
				ChainInfoFetcher: &chainMock.ChainService{State: st},
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validators?status=pending_initialized&status=exited_unslashed",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 4, len(resp.Data))
		for _, vc := range resp.Data {
			require.Equal(
				t,
				true,
				vc.Status == "pending_initialized" || vc.Status == "exited_unslashed",
			)
		}
	})
	t.Run("pending and exited_slashed", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &lookup.BeaconDbStater{
				ChainInfoFetcher: &chainMock.ChainService{State: st},
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validators?status=pending&status=exited_slashed",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 2, len(resp.Data))
		for _, vc := range resp.Data {
			require.Equal(
				t,
				true,
				vc.Status == "pending_initialized" || vc.Status == "exited_slashed",
			)
		}
	})
}

func TestGetValidator(t *testing.T) {
	var st state.BeaconState
	st, _ = util.DeterministicGenesisStateZond(t, 8192)

	t.Run("get by index", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": "15"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, "15", resp.Data.Index)
		assert.Equal(t, "40000000000000", resp.Data.Balance)
		assert.Equal(t, "active_ongoing", resp.Data.Status)
		require.NotNil(t, resp.Data.Validator)
		assert.Equal(t, "0x756f9dbe9941193262c9c8e63cede890dd407fd701744eb3fdf2800f295071628c398c7627af8c62db3df639c64c725d719acb10e2d0d69c4d01483bd4a46a1d733d8c1c0adf5d613147901688eb0d1af068153db9b157426658b6ceb92353d219a521b591d8b4463f6f157178c1a53a32e775b9a3ca6a66977c1416507b993c2bef10612320c3a3702e5493cf70e6c25b2da531cd967fb605a5f3e28c6d4464de2be28dd2082fce347631b9e34b06d565f30b4a92823a9aef721341dadc561e136cdf07230b6f5c49b77b422ce83e4b995061a4acf7951445b414cac82e32246d7a6a3ac3f10ab321fe8b0c5c0e4728cebdc94956b4224bf4d24f2a073b9a09864427cfd01b2c0ed9ea6ad8e903eeeaeba12769a90c488bfad762e98d5d9a5fdf19c73b48bcab20acee830bd9ded4f7e9e58960025daff6eb46de5ad952ca2e00b41fb6024d2b70aa8858779d4e0ff086b09a6c03b2d3f5c06db33483b877c65c06195585cdeee9f642235e42804937793585926f4c1a4ea8f1c23f50f54abc6d31d1cc60db1938996e8be42ac64c4dcdefa9a21e410092d4c939b52e5259030f614088291fe66ceebe933d309d1baf6e1a039ad7b643445668d3b32a16fa83665d31ebf090ff8cf68eafdf222ef63e9073486c3c63ee2bec2164da332280f34c8e716d150cd43cc351ea6ef7de667e8da0f10e5084f68c6d726bdb25899420792d7646de5f159d22e8c61a999d8ce378a5c1a8041a3fe0fefabdb5d44334c9b8daa3cd109923d6bf8d2a63fe0c67f971015a988a1fb900a2164dbbf32c2dec18b7a6eb6151f7eb0579f8718fc393f8e232eefc3751210fab168e2342012ce2df3a8c41da0a1ba7cd9f22c2e6a8d96133826a956f9f9ce325e273e64ea7b55a6eb94fb9786192c23007151af495333cc429ddeda8a19cd5ff43c472e2162c6d42a8e6c8d2f749f0bd6b9f836bf663c57ef0ab77c2121c5d5f01c15d8e7995924a3e06c8c6fd64d4e6f1083617e02a4eea31ac1e3786556b291aa05ef4e4e6a3a9c8374d9e3b4f6a0f56b630e71f66d34f547a96a6c0b3ebceb772deed6e622796d4ec6f04dcf6b9964431e88813efdc41463221429a1704173af5f8b8488d69156795bd8b4a177c5ec97380e19f36addafbe8bd4a57b8ede8ad0a0ca4aa22b6defb0c8f33811a82b518cf4326e1bb12d889c0c2d67aa50bdf3a7bdea1e571b4659983f318de69662ae32f9531dba657095cd9754dec907d556cbcd8652bd69776dbc07f7cc3e4cc324417f1a8efae90b7869cce98406c66f0c665e61389f788c7d6c0069a93966d9e7acd70fc4b63ffa4c7beedd251793e0a7008f75d746778f5eb14d09ca2375112dfd621022098d5d989a19c7ded627df9f572f72ebb165800721a3472afe7bd5f5e512d30124f3b315dbb1e35a1639e70a2a4b25fa8e2274d4d1b50174d8062672aeb7c99844c374a85802027d098c53a614d8a080e3f922554ed4d2beb47196d54a32552d0839c3d83667c51018eb910665222bb61e67d8dd54014d05dcdef4532f61683ced0540a9b604ca588382a362c9ac0747555001b8281e1911bcf9a8f1f575b08e936814921dbb7921604981d678428553aa939097d7fc15599512a2a973a01ffa80a1d498c581d026276e6a81dfe93387c1063e1fce2cffa09a6de2e41145a295a754a15b8f8227696fdc67755fac05edcdad2595b94fc8f52132a0e3b00e3b14324022beaddb2887c08d4bcdf628799b721976b3951fde0565f1a882ec2b7205c382e17f57c28972da3d3575a2b0ed74cff9e6c225b7f0e6f93e0fc6d802c0bfbcf985e06857040e0f1cf2560252f77f28c6b36ca0c81c389383fbf0196c57babad31320eea09d4153ac69c0f2853ad299d5759b09fca6b9541e17a5aa4923688b6e2b6b93119a5081600b61ded4b7367f10234d3dddd02960b5a3c87423d5c8e7817e2d2f33b529cfe4136e93a8d3d03971acd32544ae99b39136eb595a79c22ce75d522cc95625731335e7ef29c9ea096881988d1fb4ad8bb611958247c8915b335bef2be6a573df62a8afc3f36383a7e669589605dd394f7a920f4f9f0fd4e713d82dd29fb50717c8fb2bc6bf379bf6c0b0352636febb738e01fb79a2e49de2cb96ccbc93580e14e0fdda035b415580bee17d54f9814e3a6160ceb276a23b66072e7a4286a22e6c1dae52116c600f84bb7d85e3f9834cf7406518943beb42556b72ebf971969170d3e5c81e67ce1193fb8080d2307656c5d822b656c597e84e6f0dcfa63a15b9831a355420e37a12fccccf172d9ac23bbae7041c291852413c61cefbc70ddabc2fa41cb8c41b3b3ca54ae93d27e3dbaef5f489c1027820b0772fb88b1b2f47fb0444aa329b4994eea1dddf2bfbdfc6f23e7ce6bd9903f69cc7af9f255cac073f7bcba7e887a92212a37659dd3a928de60b69108d1ba7143080e436acc492e9a2bb6e82ce3a51563baf48bff09db0f16aa3d826e1b86fa6b4fa1523627d95631b01ea084c559a50abc876a9d889593cb4653c16bdf87146bf11b13cbcab0dcd10aed1c033e7679189c9945268a1f1285e825adde0491e2d3ed36d1a9d0a355c29002ee9e17f9d9cb84d75d8b03f5881ee6c74bd039f089153db5bce2becefa1b501bae47498cfcd17f998fb1043d36c5e95caca826ab55b30c9e0a4a77dd30522712f9b492706f1d447ac2ee3ec6ca165996ebefa33f94a7125d89418d5f06e183438a8f9987cc8ba45864aace6934291267c70489aa1069cb9b9b8742f220ea96c2eb6e3da7c3d73ee22d42e4911154d643e1e34b0fdbbdbce4b664989711abea7d1f6b2b5c8e2b081092b5332a02917151176c55176ffac81fc9e6b2b26131187eb4a1a652a34ea5266818c3d06db75621bf598cb42498ea3609b42e3cc47eb78288fec8ffdc57a3b9a599f0266cacee9912bbd6aabedf4c927d0e3b5d50b7dde3049c35d28dea06742d623b7ce0f684f4a4c785ce22be5d1d2ff53847e96016bca4e0c222199a91f3f3122a2d78997c86feafc67db826ff80e67b59afa442b29e1e8ca745b73f9fdeca11cc2097f6c1b1d413e8e521608a226dcb04597a3eb8d309ab933ce0c7f8c5e4288814c1c00284ee1960d816adf9b8216e3d53c7fb420540ce36eafd9a5e7a3dd40b5112ea1389c38a016b87b39dd07309c35fdf4ce716f32e1764cc81d1e96a32fea583d380735b08869db9cb1db3fd9aa0b2ea7001f8f397bb03a1b5dbe769fd4bebf33a0a900fb886f66aa75231a186acbefd3e849f4a33d0dc3c18575c0444b0cd4d47e4f2749ab9010f95e6929a5efde14edff59120c000cc955a136eb2eb00edfb83212907e5013a7cc7151df7bbdeb5f27b97015438a5ea8caabb9cb4af9553e2a27ec1bcb3b8ecf08e521a7cd70c596e67f91095dd9c21df66f0c46d0e6d302763079a0e4b6eda537a52a85223d4b2129cf752910dcb15ee7dd97bbc034407629885c11654ed2e4b03d696340ac0537388df7b6c9d8b35fdef14d456a7208140da3f940e399655f927ec12785d80418dca049f39059ae240ab8f9a8c4a2901d91a84ea2d56555a57a9f45375c6817cd81cd9920523131d288045041c63ac8ef8f655d91de12c00c5c6d96793df4850e06da49c2ea6072f22764c7196", resp.Data.Validator.Pubkey)
		assert.Equal(t, "0x2c6703babd27fe0412994dc78e32a9db97f39fc4932908c805ac4e75771a77baf818bbc6605f93eb9623f8657a0a6df75069bbf4a83df18519ba7a3e57f88d90", resp.Data.Validator.WithdrawalCredentials)
		assert.Equal(t, "40000000000000", resp.Data.Validator.EffectiveBalance)
		assert.Equal(t, false, resp.Data.Validator.Slashed)
		assert.Equal(t, "0", resp.Data.Validator.ActivationEligibilityEpoch)
		assert.Equal(t, "0", resp.Data.Validator.ActivationEpoch)
		assert.Equal(t, "18446744073709551615", resp.Data.Validator.ExitEpoch)
		assert.Equal(t, "18446744073709551615", resp.Data.Validator.WithdrawableEpoch)
	})
	t.Run("get by pubkey", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubKey := st.PubkeyAtIndex(primitives.ValidatorIndex(20))
		hexPubkey := hexutil.Encode(pubKey[:])
		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": hexPubkey})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, "20", resp.Data.Index)
	})
	t.Run("state ID required", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"validator_id": "1"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
	t.Run("validator ID required", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "validator_id is required in URL params", e.Message)
	})
	t.Run("unknown index", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": "99999"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Invalid validator index", e.Message)
	})
	t.Run("unknown pubkey", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": hexutil.Encode([]byte(strings.Repeat("x", fieldparams.MLDSA87PubkeyLength)))})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "Unknown validator", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		chainService := &chainMock.ChainService{Optimistic: true}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": "15"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": "15"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetValidatorBalances(t *testing.T) {
	var st state.BeaconState
	count := uint64(8192)
	st, _ = util.DeterministicGenesisStateZond(t, count)
	balances := make([]uint64, count)
	for i := range count {
		balances[i] = i
	}
	require.NoError(t, st.SetBalances(balances))

	t.Run("get all", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 8192, len(resp.Data))
		val := resp.Data[123]
		assert.Equal(t, "123", val.Index)
		assert.Equal(t, "123", val.Balance)
	})
	t.Run("get by index", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=15&id=26",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "15", resp.Data[0].Index)
		assert.Equal(t, "26", resp.Data[1].Index)
	})
	t.Run("get by pubkey", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}
		pubkey1 := st.PubkeyAtIndex(primitives.ValidatorIndex(20))
		pubkey2 := st.PubkeyAtIndex(primitives.ValidatorIndex(66))
		hexPubkey1 := hexutil.Encode(pubkey1[:])
		hexPubkey2 := hexutil.Encode(pubkey2[:])

		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=%s&id=%s", hexPubkey1, hexPubkey2),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "20", resp.Data[0].Index)
		assert.Equal(t, "66", resp.Data[1].Index)
	})
	t.Run("get by both index and pubkey", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(20))
		hexPubkey := hexutil.Encode(pubkey[:])
		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validators?id=%s&id=60", hexPubkey),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidators(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 2, len(resp.Data))
		assert.Equal(t, "20", resp.Data[0].Index)
		assert.Equal(t, "60", resp.Data[1].Index)
	})
	t.Run("unknown pubkey is ignored", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(1))
		hexPubkey := hexutil.Encode(pubkey[:])
		request := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=%s&id=%s", hexPubkey, hexutil.Encode([]byte(strings.Repeat("x", fieldparams.MLDSA87PubkeyLength)))),
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
	})
	t.Run("unknown index is ignored", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=1&id=99999", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		assert.Equal(t, "1", resp.Data[0].Index)
	})
	t.Run("state ID required", func(t *testing.T) {
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher: &chainMock.ChainService{},
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		chainService := &chainMock.ChainService{Optimistic: true}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=15",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s := Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com/qrl/v1/beacon/states/{state_id}/validator_balances?id=15",
			nil,
		)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidatorBalances(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetValidatorBalancesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}
