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
	"github.com/theQRL/go-zond/common/hexutil"
	chainMock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	http2 "github.com/theQRL/qrysm/v4/network/http"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestGetValidators(t *testing.T) {
	var st state.BeaconState
	st, _ = util.DeterministicGenesisStateCapella(t, 8192)

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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators", nil)
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
		assert.Equal(t, "0xb755417ac7b0a00d7a04ccc9ba74c5bf46213704ae6c366176b5c92dd6c209331a23b29e4c22f8db4b4c8f90c69a6e6a14c0ecae5abde6f6e6f03a41154ef97d49f55ab23e1e421f7dba88182c8ee507cfa225ddf6fbb7e5d5331dbf21995313bf40d127ca8889ce06fd10b848f83bf6b0b7412ece6255b895833c17309c39af542eb6a12c61aa788dd1dd054c8c6630e80e34c211f107c7c440342f3a434698d6abbc68bb80e98694f720e996872bd049ab12d263ac63fb63ac100c5122e1b69ae2bec37850c38f9db50928facb0a7bb39a4205eb1c7f2fdabbeb32edb17eaa9fcd68f43eb56c66e829ce95bbd86545e22f138df8b48816543622bfdb0fb36079ac56cbbfeffe6b33781c410f51dbb2188d47c924e0d4c2c8b55cf37e749d28096df5cf12256b0d62324140c4bedf3fdf75360c6a00e441a77e57e5f26e9a5786adb75b10d5d8265038eca3fe6f52568ca4099deac108604f932f793eb6e8962ec4372a49e288c488f94096b2fbc32365be7c24588dbcf83a963973a3f1ebf151fe3a36dad00bff7e72af525bfac2422a0431988289f10530c8d069c7dccdb4ac5b68981f7909f15e5faab4546d53855ef12de6cdabc1006ded7779dd91f0615621b9f7b2a5945dd044504e7fdec80c62c68b16179ff628f9f6c59398d9dfa35fcc2a40022f9ed19cb726c3052076502ac5b34ae945065191a0161526e750942f4a4a7b05695fe7f3cc7f4c22a7254063f78d80f5da1d9b737a642a59ad16d955e118a1b1e68e7fc83d198780cb37a84da1095a96c1bef28e3cb8561c4eaa05647a523df7b1d39695cd5a47e51e4e75d865dadefb2933404f684422400de0158c1a8f2dabc8b87ced747de591a68bd0f9aadbf90911cdb6b08f77450454d005129c10a9be844be3ad848b0a674356c0d31b9f19acf45a9781e85bf1a6a2b88999313bfaf816637b239d4fced92293082f649dc3c48ff0443505e63dd948f53dc867fbb523faaa9b6f95e6193b18b0bf3150b35564acc654ddcc6c8122fb134c4acb45a6f583ec5f8374b5b118153271aef87b4986d76f2e6996523cfab3dea3f794fe62c1ff5736fdf9d13295c8d3b29fddc06200aae6383aa3b3e9581eb48131d27b834c0e903e9d1e553afb56199ea789fd475da516b428d655ac86662dbb26a85da97af5dbf166a31cdb6d6dc0cec8137d75ba3e892a3217beeb4e3de845f2e762ddfd7ad1c1cc846dfbf0b75590268347a6a5566599352661e25149058957b0363e82e0e6b707d72d2ecb4f7742333586aaeed28a52f905a111abcce4fdb3950b6ee09e59ef42d5e156315ef6c3a0b3de5d0643b5c5b020eeed54d2a8e619fd15e35ae182cda8415292e40e52af5a6434187af39ee73a5ddf70d3db69de5c54cc24d1e42071f39ef150803d37550a34a4de0bcbfb02e668f7323ac7293e0d743a383d9d6120df38db45b38542e0eb3fb2d73339d523cc2f2e35f6fa2de260de4c8191a242a67cc08f6ee7ed4546c8c2c8883516f1c21e5432a56850f7c1ab67fda639424d5d5ab5ae5fa6b54879ea669f9933874077936d9a6023ef21a270adfe2ccef04ee076773e0aaa09af869dde0b2db2e7a2ee9172b5b39510e6bc24e8b4efd3c80162af808bf8fbe42ef4d2a222f825f64ea313db470905f93cd3bd944773354645845880eadab41c47f2a40abc3d1e1b1095e601a11ef1a2e00edc52b500c9cec70fea2d863bc7182f276bf0ca8b9c3bbd4e8cbff8eefc62b8bef1702681967d11c3178faa42baec7752f3bc9837b9e7b2afca131ed01c8cb54465c79261d4a7ec9407c1779457f1e5af2a76d98f0e589f70fcedc1b6a612000a4224d5dc3abe7197485c5b81e6cda403b0a8e974e1ea3c0f327f55a87a7ff063b0391ecdcd57ecef81d3f66063223fcad66fdf79c9f4bb823cf52aa9f7f141bf76ba05e6a8d2e5cec872a59285a7e90dbaa2c931a26979c60f6060e2f5b458f44d16c637a7d4508ca233008232ed6aa9c0557f2ac51fe706abd45e04f5b812b19f427870343c323bb1e7a98ea7fac273444b73777956b84dbb0de64d623211598a7d1ff169c4445469a4b0e50677ed828f60ea4f028b2aed7bf5e56a6ae7607ed6c9b291a3c3d76f1ea5a0419f59cb91c89f07d9706ec46c42fabd47b280c7685355f74b58ac10599e927ff63d5015f2a8e5471e3b5afbd208fa761f8106f9293e821d483b1da1b298a386ad82459db140ea87e1a3a23a6426bd814385a28c4c3a842a44e2024e486b0b4644658d73236ceed421de204656a11182f21e94d66ea9c290249fc9e374cedd4e67b0969acfc1f4ecbdd401ec936353309134becd14db6784a51283bda7980292cde6c8cd35ba0b3a1109905ab8b8a2f90e93c6cf47dd16737894d04426c9e3b85191debeb9302511faedb31a21ab3b22a90aec1e22985757e6ffccbe751f9c6256d69bad6d6e1cfe989202e87cf2d54f1b4e4163902d6e0df22f3accac0e28cccb4c0ed17652bdce8c5aaa3160ed20ef1bd455119e00e1c3832e6c838a6b7635723c4e078b8d2df66f9c3ab7999cbb6dea42f35ab3ea13b6d0a0a673615337da54f2ab1bbfbf3b0912fcc1b73798a883281a8a831938c02e226533b38d426204ca082e9a39ed8736d620465954ffd5a5645aff6684702a08569534713ce8b9be795a979a947f67225a2dda27d20e4f906e39738a413233ac8b5c7da0cf59caa54ac89b7002a0b73997ce1879949f3f5d37a82f49047e6c6487351eaa5e7a8145120fa77c2383278ae7f7b2b169b64c8a4ac6e6e6ae8c78b5e755d42e24621ee59479c0603736647480f944c90545038ba8fb048dfd475c739fec820c792010b4e9203d1c18b908e9feb448caa815bdd8e8363ba793544ce67ec4ecdac3b0cea2acc4d51aa5fc2f38168f4c9c83386e9c5fc2e96fcab0a9827f6daf8eb1fcb8c391a7cbbbea098b6946ea5111f63f1a31455d49a45bdb48f999a3153ee37b12917e564778f7c574b3925f12a03190b21b8bb062cd7663c377a396cb99cef834eea667d19ed20d22f11e8564c3cd670fd3b020e764118cc22cabf2f967776b0406cc67f7510e10b35d7779031822b6feab45735b7121d628160783c40f3e3cc2cccebc623d77e8f4d3c9de88fe0660a137aa5f5811c873ad57115200468aaac22a768f8e227c95faa7324b1d1a81b03c81a76bf6517a9645f5f53a8f5865ea278d95d675b25c1def5d818fbdb2a7d497bc3a0645d4e362ede1b4fbe472e6f4968d92d30ed475e55205fedbdcf2ed46bc7386d05812a7cea540aa6544525ae423bfeccad82a6d1269c9c19b8c58ce6a1565d0dc54853d32962912f3c7508276e7189686ff49436dfd80c36d9a4d8b41e0845082ac538db51b5b11e2fe23756c2360f1dc4fcdbd7d2ea2e714795baad4c7aba58ac00566b52c36951be76efbf083628a20fdd828a15e0781faadf32262f09f017975dc36b9639bb705fb9e0321a05efeed7186e96d3004b7238efc015503aa15d81db812a9876870564d2a81632d30d4e09ad3b04b8cef2801c3a095dcbfe6b321f187a55a57eb42ae495d89a9fdecb91e3795c24f57f4b45d050847462505ba32e0de293f228d7452ef8cdd0495b4906047473a38a900d471930b29c1649829b030c025", val.Validator.Pubkey)
		assert.Equal(t, "0x004734bcf98f6f83c15ccd7d6bcb5a17667dc307adaccea712932231aa7079b3", val.Validator.WithdrawalCredentials)
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validators?id=15&id=26",
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validators?id=%s&id=%s", hexPubkey1, hexPubkey2),
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validators?id=%s&id=60", hexPubkey),
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators", nil)
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validators?id=%s&id=%s", hexPubkey, hexutil.Encode([]byte(strings.Repeat("x", fieldparams.DilithiumPubkeyLength)))),
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators?id=1&id=99999", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators", nil)
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
	st, _ = util.DeterministicGenesisStateCapella(t, 8192)

	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	validators := []*zond.Validator{
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators?status=active", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators?status=active_ongoing", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators?status=exited", nil)
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validators?status=pending_initialized&status=exited_unslashed",
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validators?status=pending&status=exited_slashed",
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
	st, _ = util.DeterministicGenesisStateCapella(t, 8192)

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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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
		assert.Equal(t, "0x3715cf1cfa9b9aa845df3f270d00b9bed12227058170db92f1dd5cf05b567f89ed882b5c2380fde2b407bb593e88ef1058429f9d998cb07dabfb4b0e60f2bf2b13c4847ae2f6fcae198c771d44cad52612ca29eca4f7f3e58ae19dd72fe7a006bbd1c6ce806a248997f2d11dbd9594654473b2ff38e8d6e49cd10d053846d57a66a9a50e78b9749170160da5bf4dfdc9ea931a7dc4d1fe5605602a106561ff19cc4b73a4faa5e31449796efcc102ef6df34672c6e51a58edd3a160b4c3f82d10eaf2cbe7d7881519871004b54b91aaf1ec000a1171d8dc42170928f0ebafde293832888f94242cd0f69c55f6c01686205537e384ae23b1982679d118157fcd0861fc9dc18c71903f55f0447558995a411041d89800e8c49858b1ac36d161a2fa2383e24821609d77ed4a9ed6a69459fb8f0ee8e35bea4f331c10cad60b781f1800f781b337fe67be0bf79d0fb4a796e63f9eb4b760224694dbe534fd0bebd3f176e22e5ffce0f78d22b744d23af493f48beddbe4cb82f99d531103460cfca9d9e5c7a095bccf2887e01ac6a028acc15c827cc7d522a4b584c6dfef1c745b8a6ee224b538ac3c3328152e77ce41bf9ab3775414cd305aa327a23a3d857a13adcdc4907f62d7650260420bc6abc739d379c73a7332558cd2da53feda66687bde2de1dd4b3002f96713c44134d498e7fe0925f1baa179042895452fd7e32b3941e47d2678a5a61fa012d00a299627b88c0207a7adac51d056e19f0ecb363aa0654fb1d4d360c8f9e824528554d930b0a1436334e7fe7c2d6e00d5fdc891f8321058109b6e951d82c28b42c52e7574b81e8c6b0667e24379b69aa5fd38810cea6429fb189f185b438ab8492f86bbd5f782030cd831b24a4a98b33e39294604011713708e1584b720bb92f8d15aab0d82d012292ea8cb88ebcd3710e8adde03338bd073d21e3d9586c9431bd3445c5d33189e08e9c5789b91aceae20a267061ece8b95f95fc4d3a289a477617c01a91e9133f91899114364244d79f9262b3cc359022f8d9aa9f615571feecf33bb276087920c3490d0914bfdc77e7e7e1670407126b2a304ad67cf853a378347d00ab1f76ea4ab437404d8f9d05e9755fdb8083f51d1bf3ba1f5209882d0f4f14fe189f26b87533d3a3275fd22eee7ff541ac26e619bb5325211ecec6a272578b183563aa66f60f156361e9afe8788339cc80b99a17e4415cdc3431d8b6ea287dd579f8e2ec809df41865e9069e906dcc527fae394a0cf40127feb62f61f2817c4ad10acb45c3570711d319811f3cdef51c089e2373b9defef0cc9021191ee3a664e7518620e86ca2e54a92d81e72541a3c0afa1077658df1da1d69c37e0c5977183a863595ee56a23834dafd5d535d9d2510d69e080e1b1778a4fa109cd2bb95ad8010960ab4b5a4c38ac0897de1cdcd1a44198c57126152052e5598878fa6c3cafe64709b3fa850aea95abf9482531c2fa26ceaf5de79992eb177a4b892d53ce447df9fb7c328f1f6806d35ae826d8cb59b13c274d26dcd59c4626fbef71ed85c4cbe0da40f8f6c9b684dac77abfa93203e7d8a52ae0e98c3e46dcdaf60d79938e4f912aa87b194d74c911eddef5b6a8cc664313e5a555beedb45f730a1bfeb6a02bb6390edef6f788f6b2db1d7e632bb6e4c762a2b9d65749b81a3206452c08e6c5ee829026a3352b0bf5e025a79b7f787f8eb30d210faa502bcd1c330895b9bec8b4b596673983c9be8009c363d8f990e9e4fb70eafae65c3f1653688168f3e9c4dc0664d074b0675c8446eb5d5e005c0e65eb8392c44f6fa668568d5dcc16d666d04c14972e4170dfdc1e7b51eddaa7d59cab3d007f882ec678c103b33a431db03d2d2b9b811ea70ecec3e11b37f7a64b0636fe784c80ff2805998e102abe3fb3edd2ce475faa81443d3ad88c3185b5e3e52901c38c7118a0cfddf9713b48191b7a11897fc2a9aa78c05d2e34add1ce4ef9330cbe3d6cb7db170d348e91e233b170c6b3b9db657c50815123c6241896dd9591762cceaab24a4d1393d05d2e2390bc7c28e3be8f67f687015fe8af697d3df20a42b2ab07d092d6a487d1b2ff89bb28a125a4218d10cadedc5124455bdd18d1c165c41419bf8ee675d5ec394b65b210d66a6b8bdef9c2a6202886dae858c34202b04f942657b92fbcaf827e9e05ff269d6220c12e54c9f73653f1acba4370af4057f1534b6735c289353162354820586f2ec0f2b6892ad9f07720b30d21e55bbac8bfc946e92a6e96f8f581f5be8f2e3d26b3dbdebaed86673ab592d223a8d9fcc024206a6ec2ede35d8fd17d4bf2cecada37e80dbed9a52fa1b071ceb65051f15769b6fe8a4a74186c93518b0cfcfed41c353ada2fda9e0bb7e221d6b5cebdeb3e3d37f6d6c2f283bf2cd5cfa4fbab4b81a22206eb6adcc3d830a03f6a2739b8254a1206747680fd8d934c5227af775ac6a3a402a33ac80be5305c96133d1a2d3bd3418453e6f36e67fe6972d572c5dfe169a5dfcaa7d2ac2be576afe892ec6ed7d53376987f0b675918cb44074f994e6f309a7b2f3cbf9832f89cc02b3b73410218a5fda31d02c4386a2afe7c1f56f48ae896646e6b828a737850658d0b292587e6191695488e3a72a8189f75767c9acb7e530881afbd2123398e6c61dedb3f856f9922d98e496233c831c4a700b8f083d248b7a1e3ac60c75f2a1b64c82f95c2c3bc9a02f15441084f9aba64c33551b6629f3e6e31f0d44e3c2c9da407b4367f405c43fbdc9721f2157f425d9de6047127107380adb9b940cb86a0449f2e6a7df1adb47fbb0cbd9d5fe171e38125d31dd056302cc0f0079de029a8b731ea6dff7434f3c876e3049db9b2a6bf62d1a4d9ca4e2e8956463b146f719cfbf4e564851d722efde5f0422096c34de09fbce4de8a7216b41817ba0f34fec1ff89ba99c505403d83646d285f0ad5268349eaf74cd606900ce93a2644d24c88d334530c08273cb5fc206cb14b4e28bbcad35cbb783ea96757b7d32273fd952b77f59e06b762a167c35631649d9507ad9e72b616d6f00bf49c03d932e0034e35efd0889b9dcb0cae277f0280730bbd680cb6cf578239cbfec2f9a96250d3d2faeab6850a25cf741913e8ad5c9a6519064c7d8ccd96e8d872545d1b0887475a657a5bd115a3603dee3d665e6a32334c1f1b8ceb533fbe3c4f607ac8bfa15c202c7b13ff15ef81730caea6819d8f93288c0dda5b3efa777b21e8f636de7c506eb8620268ad0c04344709e140364de12983734eafc16eb376a40f4e211d8c505659b374bdebfded414468b369120de5b74ebfbcf461427265d0f13c1d2def926c08cf7d0cb837a9be53152829806643bbfdf350d0b9015290a65dc95a01dd09bc749e54a05625372ddff2bb1ca54e6c717ea0be62568cb0ba0da36d4e862608a85d30c9b07c9f808ea680624784c48e5f96ea9b7711aaf69ca7f551223ec91c4eb786b3cca547036b171a2f91eff17f6658f8d7d3b4eb99825488ff51674c7d5adf25b3b996471dba88b3d7ad5165b741a8d0569828e6b64fef91305ca5be8b4b2d4d9d29fc9e9732bfe8428b1a0bb403df8cb0af76b092ed38aa527395c112a219ee99c07becbbf4630999cb4c2ff523c99a97835406cd5d748765b2bb19115946d4a73a6f5", resp.Data.Validator.Pubkey)
		assert.Equal(t, "0x0068c3fc2a46182bbd30ee09a15beb730477a424f825b7f32c8bc4a059037ec5", resp.Data.Validator.WithdrawalCredentials)
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
		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head", "validator_id": hexutil.Encode([]byte(strings.Repeat("x", fieldparams.DilithiumPubkeyLength)))})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetValidator(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "Unknown pubkey", e.Message)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validators/{validator_id}", nil)
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
	st, _ = util.DeterministicGenesisStateCapella(t, count)
	balances := make([]uint64, count)
	for i := uint64(0); i < count; i++ {
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validator_balances", nil)
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=15&id=26",
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=%s&id=%s", hexPubkey1, hexPubkey2),
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validators?id=%s&id=60", hexPubkey),
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
			fmt.Sprintf("http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=%s&id=%s", hexPubkey, hexutil.Encode([]byte(strings.Repeat("x", fieldparams.DilithiumPubkeyLength)))),
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=1&id=99999", nil)
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

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/states/{state_id}/validator_balances", nil)
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=15",
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
			"http://example.com/zond/v1/beacon/states/{state_id}/validator_balances?id=15",
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
