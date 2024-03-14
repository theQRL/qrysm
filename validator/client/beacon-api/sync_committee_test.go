package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/beacon"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/shared"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/validator"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/time/slots"
	"github.com/theQRL/qrysm/v4/validator/client/beacon-api/mock"
)

func TestSubmitSyncMessage_Valid(t *testing.T) {
	const beaconBlockRoot = "0x719d4f66a5f25c35d93718821aacb342194391034b11cf0a5822cc249178a274"
	const signature = "0xb459ef852bd4e0cb96e6723d67cacc8215406dd9ba663f8874a083167ebf428b28b746431bdbc1820a25289377b2610881e52b3a05c35c5e99c08c8a36342573be5962d7510c03dcba8ddfb8ae419e59d222ddcf31cc512e704ef2cc3cf8"

	decodedBeaconBlockRoot, err := hexutil.Decode(beaconBlockRoot)
	require.NoError(t, err)

	decodedSignature, err := hexutil.Decode(signature)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonSyncCommitteeMessage := &shared.SyncCommitteeMessage{
		Slot:            "42",
		BeaconBlockRoot: beaconBlockRoot,
		ValidatorIndex:  "12345",
		Signature:       signature,
	}

	marshalledJsonRegistrations, err := json.Marshal([]*shared.SyncCommitteeMessage{jsonSyncCommitteeMessage})
	require.NoError(t, err)

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		context.Background(),
		"/zond/v1/beacon/pool/sync_committees",
		nil,
		bytes.NewBuffer(marshalledJsonRegistrations),
		nil,
	).Return(
		nil,
		nil,
	).Times(1)

	protoSyncCommiteeMessage := zondpb.SyncCommitteeMessage{
		Slot:           primitives.Slot(42),
		BlockRoot:      decodedBeaconBlockRoot,
		ValidatorIndex: primitives.ValidatorIndex(12345),
		Signature:      decodedSignature,
	}

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	res, err := validatorClient.SubmitSyncMessage(context.Background(), &protoSyncCommiteeMessage)

	assert.DeepEqual(t, new(empty.Empty), res)
	require.NoError(t, err)
}

func TestSubmitSyncMessage_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostRestJson(
		context.Background(),
		"/zond/v1/beacon/pool/sync_committees",
		nil,
		gomock.Any(),
		nil,
	).Return(
		nil,
		errors.New("foo error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.SubmitSyncMessage(context.Background(), &zondpb.SyncCommitteeMessage{})
	assert.ErrorContains(t, "failed to send POST data to `/zond/v1/beacon/pool/sync_committees` REST endpoint", err)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetSyncMessageBlockRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const blockRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
	tests := []struct {
		name                 string
		endpointError        error
		expectedErrorMessage string
		expectedResponse     apimiddleware.BlockRootResponseJson
	}{
		{
			name: "valid request",
			expectedResponse: apimiddleware.BlockRootResponseJson{
				Data: &apimiddleware.BlockRootContainerJson{
					Root: blockRoot,
				},
			},
		},
		{
			name:                 "internal server error",
			expectedErrorMessage: "internal server error",
			endpointError:        errors.New("internal server error"),
		},
		{
			name: "execution optimistic",
			expectedResponse: apimiddleware.BlockRootResponseJson{
				ExecutionOptimistic: true,
			},
			expectedErrorMessage: "the node is currently optimistic and cannot serve validators",
		},
		{
			name:                 "no data",
			expectedResponse:     apimiddleware.BlockRootResponseJson{},
			expectedErrorMessage: "no data returned",
		},
		{
			name: "no root",
			expectedResponse: apimiddleware.BlockRootResponseJson{
				Data: new(apimiddleware.BlockRootContainerJson),
			},
			expectedErrorMessage: "no root returned",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				"/zond/v1/beacon/blocks/head/root",
				&apimiddleware.BlockRootResponseJson{},
			).SetArg(
				2,
				test.expectedResponse,
			).Return(
				nil,
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			actualResponse, err := validatorClient.getSyncMessageBlockRoot(ctx)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)

			expectedRootBytes, err := hexutil.Decode(test.expectedResponse.Data.Root)
			require.NoError(t, err)

			require.NoError(t, err)
			require.DeepEqual(t, expectedRootBytes, actualResponse.Root)
		})
	}
}

func TestGetSyncCommitteeContribution(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const blockRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"

	request := &zondpb.SyncCommitteeContributionRequest{
		Slot:      primitives.Slot(1),
		PublicKey: nil,
		SubnetId:  1,
	}

	contributionJson := &apimiddleware.SyncCommitteeContributionJson{
		Slot:              "1",
		BeaconBlockRoot:   blockRoot,
		SubcommitteeIndex: "1",
		AggregationBits:   "0x01",
		Signatures:        []string{"0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc925137900f1f7f58feb511fb6b9800916b6c7042cf054ff6399362190d9e2be45586a72ab04818ad69e14b5f6428df64482b92f6a4e83af0f22a4f5e85db875a3c9c4ffe4372b4950d40d88a12779656c9454eaa01d6142ab9fbd8adb8f4ec7faa88152d4c40e849bbac1241f128d7c4d46907f0bbbc417a8bc6255cd10d306a42c022242f9a6ea5241a11d511a78872431d3dbab7275830c8fcc30abaccee67aad87ed1e97a7c611ff17213b4ef4c720198adbc39345808835519a473dbaddfc709369570309141eff0690b130074a81634b054c1cb3e9da3bc0489bd4299bb553021d42537ccf8bd09d5f36dcdbd1ef956d1c35534e9252c737cdb98ad4f70b52651c450d20583f697005d881247cc2927f36027fcc10f7fa841496502d4137b110dccc303eaabce693a0471b1ffe369a4dd4fe718721fd47a1c35a59d230a54b09becec6d6582b5d617aded2955ad786a24072a5ff54e70ca885a923283fe64cf0305034c0dc52d92cee1e9333c5a38e1ae5af60ab3f9e801ace98ab069b0fbc18600aaf121cf4bd74af25c001a452c2b562205a45e9fd71346d1bf8b7f56ff6d0a3acb4101b3c625c5d57d576a3c68c0db381b46acaa0adbf0b3df9f2e82ca346361fbed145d46012daaaeec0d6605351f1d3fadf8881d2b1d0503a55a9b6d62e013f9d6c3b2d6c69b2ea7832795374eee680a3938b671b1672be12bccab41bb3b236c11323d3e0846eda85dc8d012f0ac4ce7c9e8df4eb92ae996d4829a139aa56b26c263d5cb6de5a174cbdff23c98a7880c89d199e56148cacc03e6d37b856cf470573d071f5049f7f5641e9f01da2bc3e8b97afcd5fabf8791e729bc37b0f0f0f832c790b4ac88ce30e72f22ecd2a2d99b4d499c1d3991014d976e111586d4c141a32fa4e79a17cf3697636c59ece30858644a64aa7ff91cec132de6196b16c1d581ac23fe059ebfdd012f2a5f5e2177c914140c19f4ba90d744ee01b39ac74494e0d80efcd9ff068ddf576c3e1125daf5b7da6a3de542983c02f74d584354881dda25d97662b6e494d7dd30fcd3a8583bcde25080ba2f599784923bd7e322fc582bd36a6ca8efd18f4b0d85e526b96558395c72b3893874925617c88b5bed1f159f2b779c10f1e020ecdcfe483a58f084fdae8b6f87a9b7bd3cedaec2915693612e8e6a20666d814b01748b708b529a6392ead7b9568ea5188eddc848ac40d7231abac559b841022a8fde966290a6a233fe77cdb05ee46967004e6993ef666ad07da213e2767cb132a2c4d544308d95f02b86abaf1db3dfae541eb2a4d163a18f123807bb4d8ce674f58eabebe21e8c155d7a8f883136af2567d77e35ea248a4eefc574089ac0aae4c69d36485ae6afbef2235f383efbfa7c8eae6aa0cb0dec23fea98c74dbbe69fd04ca2ed4a8c3207b178abf39b217361c6c981af38eda178017ccedfb6c42b3b5f52645aa7399197c8c4cef8662af996b1483b03fc0a8d128bca221e9ca5c93a9c4de167c91e8342d340fca343ee5bae69115a914757c077957e5052256e894e481177834d084a645c7fa4c7d885bc8e33d7b7be9953fe6a8a98b6d7943579b3dbf23c3d4594c93b951a7f27b112a33264844df1e0600857b30e5863cc3107a642988ca210db0ec09adc554945d1dc3dda6b9ce7aa7747ed39f3eff0ebd63153f807a017d5ee1b5b5358f8434ce7c8a65df9517aae2a4582d15e86cc6f14cee3b6fceb9466c76f6c8ef9af0cc9fde8c4205bf8804d4a5d916b8ab75c9a3004977d7690b870a8f7292c8c515c56b4887c4431eabda1ea7a3f93acaa32afa5d0a416c2411f1723e544c8e76f470e4a4099f766428b54b68001031fdcd1b2d18bfa72c1b72ff83a44b8ab697d289ebadeea0a5cf0c308a6f8087635ed7df1cd6cf2057e5471f73561f8b6e888388f128766bebc9804a527be652f804e4c176f89d540b70359c2240ef3ddd2969765fb00014996ada33057d4ab844cf4d5b1f1c9cfd51c94583f23bbe5ce1f467b11fff9ba4b3f64cbf76825be4010cb7567072836526324fc7602a713f92051bd15cbfb6f2a01bd3d257e77d08724ddc708ea10c13922df5edf96963eebdbcdefc06118e6fb04cde68f11dfbb69a93aa3889b9fb6cca907f085a52bae555f721c315dea80028f8016d5e203f1b266c224a5608617e1bec9eb3e19338b8e970863d77432bc8aad37052a6f8c14f5efa387d24b4785a9db270e97a500f7d67262111363932778e823ba10f5c723d427d0f917222a8777f2d4af05e9668fd54e70e0dcecaf10f07d81a5860efc5bea3a3f398c7bb4558db6833270e618e8350a3b6a43a7a45e3d13d3f577b06784f535608d804b237b322e020911cde3f5841e672e5ef16cab309eaef0bca69b000297081f3b83dd29c2bb5ec5c673d6fd022a7ae2aa58c609f64cda3dde7d9e810c0e3c151684570abd1246cfd2cc4aedeea932c668629b471dcda1678b60f932213b67982e1000bcd830396886fccda9f01fa33c7c582a1083023386a9d71a7c7767ad9086adcf10c86263e1477a13b32902334af59115a422e064158f9ced4a536848762c3b99780aeb14b780938822dd9c08cf8a8df9accd7796a9266a26d1bfe098fce386b933a5a9f50d1fa61ff4f778e772cf9db7fcce913ae76342ab189d3f0a5a3b9f41c3d6a0128fecc156b97a3459c2249359e5e6387066dd2095ca2836768be8b7e7525a855d28e56481bdb808910ea9a6db004ebcd959e3ab2c60d174f3448933038860c126c7121f04220804bf7f816c5ccac37acd86c5252c657d29eac45ec8c0e64da58d6c9e430a8c0f8578e98d29dd45a7e6ce2b6469a086b7dfceb550547ddcacc0b3a5be5491b4827aa6ef9aca1386b51b2be96a05751bc69154312a21f60069f530195bc5018c14c03fe47e749bf1f476dbfd63235aeab1606526c63e3977f03df6b601a0f8ef6952192ff29801011ce3f809f885b0f46700246c1b53095d8ad98de20c62687fa06f391da2d28e387f61115d332230dd5afc12b28479b8223f519f3042f96a6c49255b2ce74f2d4634c32bb67d10dedd56aebdb1e54bf144e4a429f10950c2b1388f3bec123ddfe112f69c785264a09352c9f3ee436dac1e2ce5e3413e14e47ab1cd178a68b97f99fe933a353e7c8b669296d1ba50426878ffc3a1d616a91964cd92917731108863a70699b11cba6b4c2534edaf3d7482e3e5e6227c4291fe34cb8ce3b4bdb62e37d0a9ccae2fa01c23593b9985b3c2c5eb38b0be17850f244f9f8c57687fdce2f9ce79a371662e75bea4947732c958e30d84f0ce6ed6f4829bbe83e62121ecd3307ed15c73fd45cc3a06d632e34aae6f97e9255d9cde21ebc6dd57595b518544007bf32014aa5cb12cf667ff6f2caf8f9aac790640a9c509afab7ee5335f8421efc9fcf408bf70c270081ebe09b29ab9089eaba87ba409376a887fae7136c79772e961dcd45f8d4df1895d5bf109ecbea093c502ec0a80ff5746f69fff3e4fcfee4b5717723c4946c1d0e4a0e39262023709534784644aba5a30b7fa619990e4aca478abed8ed39ccb4b0fac26d112801f61de1cda3abd78168bc0e79d7929d7a0ac48aa86713c26203917aea92f8ea3755bdcdc95f26c8ce9c10729146ca5c4ab93cfab4f6a1c2b8c48b324966b8d6d3f998a862a396cd8d5ba2fc9b9c213c83f8d5fa2a48b93805096eceb3853e4210c81ca1b31d54a6c3f32083524d95e905c2c5e1bbb8022bac020e01e9dd16b812a340bd982dc2e442c8d816e32263b1399872e2ec838fbf86bd564aa463e52d270e8f2412a4c5f9a0003a54d5f0d6a35b73cb4b46aa78417d7e13b31ae8fcd451b5e86bb33b27fbbb0722d428964d4610b296eb4182f981f724036d2ce7852b8c90bbe146e5a0c4eb1f4371fa238ddea0a247104f5f7f7d787d9ea07319be4570f959bb8b1a0cf1d01c2402c1c2d91066829aa09af083f2e4ec2347d48e91dd4429ca97f84770afb6afbe8ef148a32be619daa2e6a648a20b3074b35370accf718ea4f85d5b9b771a7ea7a2db131fa757625a21029f3e1bbf9aaf294a3dc2a563ddf0cccdbca8c3eb70fb9b6919d7f9138c54f1429f581942dc8d3e400e399b3c9516ca4cffadb2f9e5b741085b9126af95f629a930da02c4265b9ab43123f755fe8520246b3902df0be0212a73c6f29b69ee20265aaac513c6b0fceb20f2111015ed03049ccb058e20248decf1979a83c276d9bba9bb217e584d953b970ae32535d2ed5e00b0450fd461eb529fef32a2dd66075337e6518428a0a68a06c6f8b6274237923f320b363b75e27117a32be8300e9820ced7c956e87d5bbb202f3db79f8068454eda7841c13d690d34d6603e1131da5a5c1a8007649cbca5583b52e5c37da0746aaf65e2c2bce09df6df2def5ccd17f96899bd67695cba117fa12ba7da1e8205b05e7c99ae6d52483ba784e388a8bd44a4d970d983dbddcba00d6e597fa341e5a7b12117756874c0ee22a80edb8acc45bd9d8e3e38f87313e3fb38960cb8ee28011515f3fde8b202bf4c7ff6a6ea485246eec77c6040990755d3b62db9c40387ce8dee97e967a396802a12dd27af1588b860a4b3deec3436a4dd497d74406e01a84b74806403111bf687d9b851ee124ae568d7513589655c86f5a12d66b36b3fc38d55422a05e622077fb54b140a5b56d86896a3e372688eadfab7e6d09a01cc12222c85566b7ff27e3e6ea7c893154b247021aa904e9d7c50174ef106d712f2a2c937d228782ae58d546232b23dfd5b103fa1e288bccc7e475dd8cec1713dd04651d1b1371b983a49d9d523cffd2b804dd813497a40bd7f51e55f9e30f99d4cb26e6f48dd50cfd1b93216ff34ac5ddab8338b7da077a423fafa146fc9c5e2504134e08975746d904467f7a0685f2710a4f4aa50c5e6d83598d0ee91f325db848bf9421d613777e245082de907cda3c33c242072413fcf94218b1f9e79e5773ed5cf864f0e88f5ddb6ceb005956139b23e4342b0b60934abaec95491a965d0df446e50801554668a8817de12613ffad17bf417c912489444fb0743b2f56020c61dc7facaceaab7a80bacc77bd1db1ba12e32633e8536f407f1b51c8e868f2e9f61799e20948d9a33b076377a6759e3a2de62f453bc726eea49c67adb4b7d180767b1d046ea8ea4b475cbc9a105514285e632be6a9093d8fdadf05d446360cd0b71717821e46ad3aee2a61183c3b065e874fab8b8ef98bf4d8862bacf698716fab299f8adc4ab6fa5a4c02c4e577f84e62cb2222182fd8e0fb115b4ece1683cd27350a7565058262be790b4e78ad733d05f2f75a5544c40e56fd48dffebb402f8a37fdadd0d1b6d63fe08a75915fe8c63ba675d72cb3fce92b49092b7f36783541a0b140838d3a9053f6992d1cb489a8fadc089669d21d9eddcc62cfbc95ed45f0a6e82020fbd0575e8e1ed72d5f984b2ddad0d5b2c71534f03ecdf810aace2b0846b047c4756e18dd80b8e30c4b7cb2acf063624707120802ecf85fe3b0cc31e836f953beea6248968d2f62b80f730a8903710993fdd3ec0c9ad39663d51303a15cf8e8cdcd063732a0e3e1ebbf7f6b1791768bb68131f8eb59f3cb2146bc578c0c8656eda001531e1ccf2dd88053de0623b2080c5449c21edc1cfd655595d2c7c5114366d0012d75a1ab11d438d9c3ec65811e161e982c9e1f29e21f6db1d46f3eb9d737801dc82f27b9b92d415baa3b710d71ee26ecaed28b56fdd0f5fa2151fced422bea17822e486f684912678793c244c9c41945ef49de13a5b41b6ed277b869abe2f44bb40ec2d07e23a4b4ace58cf06a580f29df4531a8ae1240b6de60b542f2e21fc4de0968fb9b1f2616b303768841495acaac5db2a53156a4c60a4b52f9ba577e66878f1c3d8f886d1badcb170ed506260077e0c11b9712ca7a08770f2a6f4872ffb37605b98ebaf4e630052a0f18b043ec4ebbe60d027fddf1a6da5e8b78ac0e133fb666c984be49e2d8b7044b58076ee16c42557387214c4284d72fa2d4df9c14f5545cbd0eb731a5f5822b38c9379a63df3407b181f9d050e48357ef209b5027c1a080c5ef9b2d2a9a1acd06481400893567f3621c939ebca1f69e7a05e67a6b70dc98be12fbad244719f8cf27f04bedf2baba6f872b8711059f18fe4b495f50410ba65ece350d9d33f526e04099545551fb0136fc68b968953309a03be33e459c6c399e2c36fab6d60266f884ae1ba0b6657db29a93ee73c206428d6a6ec140625c277f67d9cd30597ba85405592a9e48dd158e5008607391b496027aa82173f5e70b231e70cd92bde95492b3a6192a55070ce34b6c4dd184c4f7797c50d2b354b4ec0cfdd03576b868ea6aab3c80a49686d23253f7fb80000000000000000000000000000000000000000000000000000000000000005080c121a23272c"},
	}

	tests := []struct {
		name           string
		contribution   apimiddleware.ProduceSyncCommitteeContributionResponseJson
		endpointErr    error
		expectedErrMsg string
	}{
		{
			name:         "valid request",
			contribution: apimiddleware.ProduceSyncCommitteeContributionResponseJson{Data: contributionJson},
		},
		{
			name:           "bad request",
			endpointErr:    errors.New("internal server error"),
			expectedErrMsg: "internal server error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				"/zond/v1/beacon/blocks/head/root",
				&apimiddleware.BlockRootResponseJson{},
			).SetArg(
				2,
				apimiddleware.BlockRootResponseJson{
					Data: &apimiddleware.BlockRootContainerJson{
						Root: blockRoot,
					},
				},
			).Return(
				nil,
				nil,
			).Times(1)

			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				fmt.Sprintf("/zond/v1/validator/sync_committee_contribution?beacon_block_root=%s&slot=%d&subcommittee_index=%d",
					blockRoot, uint64(request.Slot), request.SubnetId),
				&apimiddleware.ProduceSyncCommitteeContributionResponseJson{},
			).SetArg(
				2,
				test.contribution,
			).Return(
				nil,
				test.endpointErr,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			actualResponse, err := validatorClient.getSyncCommitteeContribution(ctx, request)
			if test.expectedErrMsg != "" {
				require.ErrorContains(t, test.expectedErrMsg, err)
				return
			}
			require.NoError(t, err)

			expectedResponse, err := convertSyncContributionJsonToProto(test.contribution.Data)
			require.NoError(t, err)
			assert.DeepEqual(t, expectedResponse, actualResponse)
		})
	}
}

func TestGetSyncSubCommitteeIndex(t *testing.T) {
	const (
		pubkeyStr          = "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"
		syncDutiesEndpoint = "/zond/v1/validator/duties/sync"
		validatorsEndpoint = "/zond/v1/beacon/states/head/validators"
		validatorIndex     = "55293"
		slot               = primitives.Slot(123)
	)

	expectedResponse := &zondpb.SyncSubcommitteeIndexResponse{
		Indices: []primitives.CommitteeIndex{123, 456},
	}

	syncDuties := []*validator.SyncCommitteeDuty{
		{
			Pubkey:         hexutil.Encode([]byte{1}),
			ValidatorIndex: validatorIndex,
			ValidatorSyncCommitteeIndices: []string{
				"123",
				"456",
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name             string
		duties           []*validator.SyncCommitteeDuty
		validatorsErr    error
		dutiesErr        error
		expectedErrorMsg string
	}{
		{
			name:   "success",
			duties: syncDuties,
		},
		{
			name:             "no sync duties",
			duties:           []*validator.SyncCommitteeDuty{},
			expectedErrorMsg: fmt.Sprintf("no sync committee duty for the given slot %d", slot),
		},
		{
			name:             "duties endpoint error",
			dutiesErr:        errors.New("bad request"),
			expectedErrorMsg: "bad request",
		},
		{
			name:             "validator index endpoint error",
			validatorsErr:    errors.New("bad request"),
			expectedErrorMsg: "bad request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().GetRestJsonResponse(
				ctx,
				fmt.Sprintf("%s?id=%s", validatorsEndpoint, pubkeyStr),
				&beacon.GetValidatorsResponse{},
			).SetArg(
				2,
				beacon.GetValidatorsResponse{
					Data: []*beacon.ValidatorContainer{
						{
							Index:  validatorIndex,
							Status: "active_ongoing",
							Validator: &beacon.Validator{
								Pubkey: stringPubKey,
							},
						},
					},
				},
			).Return(
				nil,
				test.validatorsErr,
			).Times(1)

			validatorIndicesBytes, err := json.Marshal([]string{validatorIndex})
			require.NoError(t, err)

			var syncDutiesCalled int
			if test.validatorsErr == nil {
				syncDutiesCalled = 1
			}

			jsonRestHandler.EXPECT().PostRestJson(
				ctx,
				fmt.Sprintf("%s/%d", syncDutiesEndpoint, slots.ToEpoch(slot)),
				nil,
				bytes.NewBuffer(validatorIndicesBytes),
				&validator.GetSyncCommitteeDutiesResponse{},
			).SetArg(
				4,
				validator.GetSyncCommitteeDutiesResponse{
					Data: test.duties,
				},
			).Return(
				nil,
				test.dutiesErr,
			).Times(syncDutiesCalled)

			pubkey, err := hexutil.Decode(pubkeyStr)
			require.NoError(t, err)

			validatorClient := &beaconApiValidatorClient{
				jsonRestHandler: jsonRestHandler,
				stateValidatorsProvider: beaconApiStateValidatorsProvider{
					jsonRestHandler: jsonRestHandler,
				},
				dutiesProvider: beaconApiDutiesProvider{
					jsonRestHandler: jsonRestHandler,
				},
			}
			actualResponse, err := validatorClient.getSyncSubcommitteeIndex(ctx, &zondpb.SyncSubcommitteeIndexRequest{
				PublicKey: pubkey,
				Slot:      slot,
			})
			if test.expectedErrorMsg == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, expectedResponse, actualResponse)
			} else {
				require.ErrorContains(t, test.expectedErrorMsg, err)
			}
		})
	}
}
