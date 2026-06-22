package ml_dsa_87t

import (
	"bytes"
	"errors"
	"testing"

	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSignVerify(t *testing.T) {
	priv, err := RandKey()
	require.NoError(t, err)
	pub := priv.PublicKey()
	msg := []byte("hello")
	sig := priv.Sign(msg)
	assert.Equal(t, true, sig.Verify(pub, msg), "Signature did not verify")
}

func TestVerifySingleSignature_InvalidSignature(t *testing.T) {
	priv, err := RandKey()
	require.NoError(t, err)
	pub := priv.PublicKey()
	msgA := [32]byte{'h', 'e', 'l', 'l', 'o'}
	msgB := [32]byte{'o', 'l', 'l', 'e', 'h'}
	sigA := priv.Sign(msgA[:]).Marshal()
	valid, err := VerifySignature(sigA, msgB, pub)
	assert.NoError(t, err)
	assert.Equal(t, false, valid, "Signature did verify")
}

func TestVerifySingleSignature_ValidSignature(t *testing.T) {
	priv, err := RandKey()
	require.NoError(t, err)
	pub := priv.PublicKey()
	msg := [32]byte{'h', 'e', 'l', 'l', 'o'}
	sig := priv.Sign(msg[:]).Marshal()
	valid, err := VerifySignature(sig, msg, pub)
	assert.NoError(t, err)
	assert.Equal(t, true, valid, "Signature did not verify")
}

func TestVerifyMultipleSignatures(t *testing.T) {
	pubkeys := make([][]common.PublicKey, 100)
	sigs := make([][][]byte, 100)
	var msgs [][32]byte
	for i := range 100 {
		msg := [32]byte{'h', 'e', 'l', 'l', 'o', byte(i)}
		priv, err := RandKey()
		require.NoError(t, err)
		pub := priv.PublicKey()
		sig := priv.Sign(msg[:]).Marshal()
		pubkeys[i] = []common.PublicKey{pub}
		sigs[i] = [][]byte{sig}
		msgs = append(msgs, msg)
	}
	verify, err := VerifyMultipleSignatures(sigs, msgs, pubkeys)
	assert.NoError(t, err, "Signature did not verify")
	assert.Equal(t, true, verify, "Signature did not verify")

	msg1 := [32]byte{'h', 'e', 'l', 'l', 'o', byte(1)}
	pubkeys1 := make([]common.PublicKey, 0, 100)
	sigs1 := make([][]byte, 0, 100)
	for range 100 {
		priv, err := RandKey()
		require.NoError(t, err)
		pub := priv.PublicKey()
		sig := priv.Sign(msg1[:]).Marshal()
		pubkeys1 = append(pubkeys1, pub)
		sigs1 = append(sigs1, sig)
	}
	verify, err = VerifyMultipleSignatures([][][]byte{sigs1}, [][32]byte{msg1}, [][]common.PublicKey{pubkeys1})
	assert.NoError(t, err, "Signature did not verify")
	assert.Equal(t, true, verify, "Signature did not verify")
}

func TestSignatureFromBytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		err   error
	}{
		{
			name: "Nil",
			err:  errors.New("signature must be 4627 bytes"),
		},
		{
			name:  "Empty",
			input: []byte{},
			err:   errors.New("signature must be 4627 bytes"),
		},
		{
			name:  "Short",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:   errors.New("signature must be 4627 bytes"),
		},
		{
			name:  "Long",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:   errors.New("signature must be 4627 bytes"),
		},
		{
			name:  "Good",
			input: ezDecode(t, "0xb420418a391f820a2417d2d94fdb4945975e2ded6c179af1ef4db5c9e7267fdde366a457d0e819d05eef6c984834104dd85f01c7aa136ec15ba5f39ce3dceea920879050c3b553e7e7331ef58c9c254f05491c3ef95bd3cc2f93770a72380d29d5d9cbfee08d55485a0105c6150b13fb085188067566f893242f492cf927f84edb6604712b216f058aa1bad64977ea3e2af3434d85d81a8260b0e4e73109920b9f1dbaedc5e38e5c0bcf8e7a322ad7c0d677ceecccdac69b64eec75d5d6b4d23eaf91e7ae2ef166a5770fe4c9b01c11d011d10228711d54ec024d704a2848df6a68b1eaef4215ab150ab46667d29051374372913bddde231db6d1295c3cbb765843ab3b5f0a2a6f7ec49ee6095b6d4c8bdfaeb44effe386b15b8a06e75914a85b6dd142dfa8d2839e6af5e412bdfe5c89a75b838caa7fa91ebcb83a8a7b00e3acd188606c06884263f8d9142f0cfb55d94aedd820676470288e5e262d0bbda0b088177e8ac7c53ea9d0f084a1a863c60a7194b59f85667ec269d798c3b14a2c292e646a11a911214bb8504814aae1d0ceb7aa08d0769fa4de1a05928c00d4d4a24182a79ef2dbaa3969e30c03f9d1cdbac3d5b2f9617ece900b2054e36894a6e1ad2da87e699e82ce2ce43e94a396d2f3088088cd3c7f7689f29b5ce8ae4aa6ea769f32bb3969ea76e117d1983991399fb119ef4900a1124f70d34f2a008998e209d1699fd0d3537d0f51d02e390ab86b873aee68eb534d2c97cca7a301bfe80c01c0e422e6ab1bb5858ffbf05bc039000986ac09562d78ca53f8b3195c9a0a5ad32e74ce463982078baf0ced9b7f84b588538d2d65465dcf17015f0977247955801ad6ddaff407107a0575477540eceb2fc3909104ae56e6b217907e0be9eb04db11e37762e5c9b2355e0ba7e4034a4e8cc442db2cb1a1c075713e3b499d52da1fdf1c80b5f63cdd28fb6f2db1bc7f85a6d176424e40c33fa267112482022cd6877252409ca98857163b5b2ca6a911e21c8e9de554ef466196610b7b050a790f69994e8d250000d9ff70f2fc683ea15ee1377a9e33e789974a5bc0457a105c252157575bebf7f67e0e2f920d23908b177550bb694b5f86bb8eed54960d09acc0c89f2371c7bf2c0c06aae820478e7be1b193ea459487930b931541d10bdb0413869eedc18296ee7420abbc0761dfa2418a00cc651aa9a770b66837824493afea5359887173458cb7c30321badb4f411d1e23277faaed73ebbfad93b0f9f1d4af4506d402beaa4f02498353199f28d7eece77fd5a7006e470a0c195cc4f8bd09a618adc9dc599f8d03e05af64fd5f1870f703a1425ca0fc8e8eecb4b53c482300b367b17d9875eb4438e51a7b65f98b62c2d88efe12ca5861429f4a64772b62d3aefd6eb80dee13f9ff407faf3b57e29569d4964f3b7eb3cb523128a11fa3f4fe8af563c7233942ca06593d9b516afaab842d07893321026bd94807bab683b5e03baca7104006fce348a9d2289e17095239874e301ea3a83e3a25322878e280f75fcfad2fd489d6ace1647d5f6a39378020352e2e3d33cf92aea6fb571eee7cd5ffd48bb6302e2594d37168e7aa9457da759bc752396fbed71652ab7568c5e5f539ba2e29b9e525f39618d0d3be143bb143ed99bf3faffae471337c2884ff6ba72ed21030f5d011eba1aeae8369a6bd42b7458bba6896491fdddba9506a890fd556309f1c79a22f00f4edca7e9913f39e7cd4f9cdb6c111a4ff04afe7578084c83439374ca93a2637a786fb5de373808b8306390661e8c609c838eb4ff00ff0463721670648eb7f2e4b8b4a5936506b3a001fa4e2cdb6437684bd5c97b5a365ad226e7d5e61a8285db3b988d76cf43623e4c2cd98d27abf398b2e50073e12407e3b0069de34455281dec34ee9bfba79f6cefa607f3863afef90796b97d6e94db73074b4505d049a7731b84c8dcf5f27bcef9e48988e8e56bb0a0ec55cce4d2c4a934ac29f5181510025975e18d9b7791d77ce599dca80c560aecdd36cb6007267e0c7a8bb9513fa7a6be1cd42e51f81b3f251e80c4db83290fdce7b946860e0f1f92371924710f3fb94de620b1470af3ce87e5badb09a7fa3cd6c637583d90740af998245b171c41eff462e98f307c3a2606e0b68c4dbeafc8617fff4747bcb572ae83b7bec89aad32c56b29f6ca404b653a4ff058674ccd1d812b09e6c50d261c0ae72978adb59fb3d45a37a96a01b96a6d79c9b4e6d036902319305e74b14069d03696b76a41eb06e735e2ac9c5a2bb764c1d3e8be0721f97e3ca46688ab7405b1fadcaaad3bc86fcd174f7810a72daf9ca4d5303900af90c1b8348f62376de09a0ae335dfbabe399103a00b865a52e93592af97030d3d9f09c9f91d3a72e11c2c120a097781f5fb0d33c8667c5a92eceddda4e45e93e0199c4946863d55121986c67d541f34dd4238e0a3daed1ef3550a00f786ffc3435cf57427d4b968ec39e420d2c05aecb2d1c65a0e35136ae8328b3959bbdf9d19a393f4659f7dfd10c383a2147bdaba478a617a5a67a50791e88ea679b4ada141cd2a18a0f9589b47b3384a1332a1e2c80988aa482525d74baa0560a8f64be3f5469dcfee8c3e2edf6b3499300b6d8da7fa66db734e8fba0634d78e29e23425ac1c85f306236117965b9b5722abe0b1796e74d600170d5f0bc748cf54c8ecb1d4931532aea9cd0401e6de4d17dd1551943bcaff5a5c5255eb9d2e98514583716e9ea9fb164fc7d3022012827ef84becc6afdbe1b350ed6ec6f9100d76098efb34f16d9dbaea2f629c224b4fc9b62ddd49653a119e7854d68698bb30d479603d60c70d6ef636f6b37946c95b8b73dd7725884d3f4fcf6272065ecdfa245b3b4b35af068e4edd443330536bab976460fd7349eff9fd44efcfa57e368fc9896dde3a51ccbf9550ca2566e0014729872a29e53bf8ae68371a06b7ff26744dd1fa3974e176f42943e6d26f7c70763d9ee9851af79155ba53d782569e87b5392f08929c109c725acc7476ea79d9f9dfd0657181d5f92ae53556648dd00a9683dba9a48cde6f5d148864333ba8fc6f041eade20a68332fe5efd972845ff0c02c672924908e2e62c468504d5ada216afd3d66ddd601a972c962c75d19a4444af9581b26362369048a5e81baf83c540013562e1892b40da4a296ae7f2a42551d6cba4a8fa88d3ac5574e337533fbde84a8969d98d128daacb681e5da98186cadfd4adefa04d8a2dbbd57bd4c6646f49612eb1f2a7889b21999f94ec56211f14dca19cd12fc3ebcca272bda1eee52a57cfb1bc9f04e4ac55703a233f75c0805105ee0b6a8475048580345ab7cc98a544ee41e8bc214d0d5874e4846943b258609f178a81590f01f14b9673a9ca745064d261b2adf173df404623ba58f5820d1c3e85d52d499a67e0c55e45993519d7bf0bea9b13ab52b9bc2b6ce02603e7e20ae7f073b01b33a2360e5d152803419ccbadadfe221c3b0a84754ad7a4f9300559f2c0d5f55020424d0603f1defd89e0f098a800594b356747dc4beddb7b3b455dd07c13dba063384295a2cb59071928dcb04c9f291048eb59a6222a921af8debdbdd83b7e0c91614b52fabb62b82d4d84775b24122636809e6207d9b00103bacc1a843e899ca9521ba2b32e247f6b9a27aa4504ab2ba1b11174ecb1be8fe119419565d7add3af11394fcd3d0e399dc5be7fd22cdb5f254519c939b7dae5caf41746840f41a57ec18081afbee09d52a3d22792e5653c13d03015c5245a363761a6e6bc119822dad3a8e94ad91b544beb99c1114fd56b9840d27c0e9d9903b2bfbc05a4909cea3e694795fcb7ce310f5d196ed6983dae4584fea0138ab94909222f0f5e30042dcda79a46a1971b1836897c488d4e381143b71b2d7a6a4215b38a243655f65ab3e39337f6e102594b668e147c88f6dc9c5851d3f27d63dc949d51c03a164e21edf4600182146c27c77e9203008daab3508d9f05535f5dba476542ca37701571040c169ec22cf81da72107466e122d2fa9cc0e31f3ac129749cda83cb5d81863efdda2df97902483d8a3c99776a84229c44ef40a3c75503671c142fda9e15e76177c6512f1130d952b472aea7eec3863f45a0984f7e61c316587bf67f6c4e4d0d6d619094d8a9f9cbb7d7a2cbb9ee17ade2ab13bf8891b635b5a14c1fe8ad7a4ff95077f42fee11e563bebe78676464fdc1808317609bc02c99d9fd970b53518888a5c3dc1e7a03ce233c32f711e9c69a3a94c88c5184696f0b0ca1d50a2c885cfd678a8db4b4f3544ade59c38ec6516eeb5842baa934a9be591f3c4b685cfc2164e919a15758b56767f72f26cabad52d6e7dc49fd78ff678b040f15a4de02fba9192920a303b238149828c328e826aa61f53423afc2fbdcae247a637da69b55a0487fe6171e508d2f514f9b61ebfdea9800e56937cac9a50f0199fb7527b7e82ad7faca5facd7feca504d8e53ec52ec3819469ee1f60a27aa09ad347364976046e7808348556981d0948d869b01497bba9826370978d394ac04eb34af3caacd92e7bc609b425fe8632e0cb5c948d3deeb80bb52a1af2d6b4866ebb77a14bc16e421cb6386ec1cd08d758eea85ab1bade56c7ffbc0e02f1ea1b4af59a8b97832e2ef20e137b705a997b01cf70c0488593b78a2b03111d398079876e4d78eb09f0ab6d316234baeee5d16cc5ff27058a355b1d1d446fec2e88a2cd93e2484d9a0acdb448d821f5e5fdac76483f245875dcf39ab7b94313c5cf5a0365b7bdc5767d54fd396a49ec4d75414a49c14bfcec5fc675cdb0cbda22d967b4c04684507641139f57908d20634524b1d2b9e69ba1da033be708b489d2aede6f62d7f0c9cd6002314c694726a5aecf9e9cf98d2a8340c9b183f1ae341147fed7e39fdcdcc5d54c7eb973ca3984b54dbcc61e332df8abfc20ec323393e4731a5598e85ab16faa269a0ef035c3bb424914cd75c91a9c60e8e976f424eae9e84a1fbc2c88bdf8ca7bd252dc9767612ea6711d497bc3dbbccdaaa7e3f934e676ef3464af76197508843951af14c212d2a8ba47d7af64fc6fe415958db233475471440299ecba57ab9c434dbf4eee99995607c08c00eff4de386a532213ad477eaee9367881f7d93904b6000ef82c863f83057f5cd1af1d7004cdbe2a4b27d3ffdaa9fd6ef9b3c9c5173fb9231d50ac7030ea0be204b7cba41a1511123733c835e75ce3748bec5b70f616c32df8d29007ad0d878880ef75c46658ad9cb44ae0075ee0269004ed58aec72a189a1f51d597e2433ed7f29449d332894eab57950e382d80612de14d5aa2fce22a2c8324167d1990636c15fa2dfe301172a973cf7e7cf94418822abef7223223d4496ba91ba7d3b50cc072ad1c3fe729f1aa61a41afc224c9609fa745c5f9d086db5b6bc7a3bfdf6e67e2b0ebb9993fc1ee7b0a08757845d05a9d95e9689b7ed53b5870cb6a2596d5e35cf9f590401c1ede237969b77adb833dd7f1881f652495b1383faf7b80aec52dd6175bb65f0dae8488d143d6e99ca05c995687c703723d26065582daf7efbea7a43db054b61ca4da99c93f02f115d76e40cfafd7ed7cfbb37cef31ee7ac700aafe64f08c6a90c6de9fe8addfbbcf21640c1e35dfde735dae70885c06b1bc0cbe44caf6dc0b5f9b66bbee55c1c924ed2123a73fda69f03318942e045064683c2a4f3ac16cd609abfc518a0429d4c00e38cd31b4aaab3bdaf2c0a7cf0aeefc5010f4daa3850fa6b7947289b389932235eb9a9ca0ff39d1b00df75f342aacdee3b45fc872ae7b1ecaf782edf7c49e25f6f2b230be9ceff0e4f38b0648c2d0afe058741f1f73817abf0f8cb3d571a6d256f5aa75d4926f86ea529cc16a1f7c748dbb21eab17123f37e1d3edbc306716694f06e82aee0163e4d089ca44154b75e0a466da488e9d9618837cfa30e8745bda1484d600fb82f94ac83f1439f956bd2c217a1b55a6bc84f3a89728e27f241946241b235f39872f431be2d86b358cf5aa39c27c3387a74327c1458a4197c79fdb778dbb8e003ba1fe0cb8d7f5a4b59581bcef69a6a219324575a5ce150ad7d6c7798eae7681b1453f947cca0743605d79bbe0f0164182473c889d53c62962be351f5a09cea50cb8f2e96b33c854027c180a91c817fd9adab42eb354ece697289e9ac9dcc8317e39a5fb73104e8484aba3cf77bf88053a257e8b5fa34b6eff2cd8d208ef20d5c7d56319ffb817cfd5ed1bd104509e81a0a590e917bd9eca2a360a3f612ada44596624726501b8e8925fe228db6ec7b287b75b3486e704d7cfd44e4c4b78301fcaff73519827c669f1f4bf21c04d9674ba4bebbfe9b13acdd01acd975e31c6ab19e202f7d733fa6d18df68cde08863ea051e47c0a58eb695ad394e26a39b0aa69439ef9c7d10b0544155123441657e9dbecbf4133547555b6a72364a516e9fa4aab94168979cca294550559fb3b5e6086c739fb3e7effe0f151d3e547c83ba31354147a1d7de0000000000000000000000000000000910181d252d353c"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := SignatureFromBytes(test.input)
			if test.err != nil {
				assert.NotEqual(t, nil, err, "No error returned")
				assert.ErrorContains(t, test.err.Error(), err, "Unexpected error returned")
			} else {
				assert.NoError(t, err)
				assert.DeepEqual(t, 0, bytes.Compare(res.Marshal(), test.input))
			}
		})
	}
}

func TestCopy(t *testing.T) {
	rkey, err := RandKey()
	require.NoError(t, err)
	key, ok := rkey.(*mlDSA87Key)
	require.Equal(t, true, ok)

	sig, err := key.w.Sign([]byte("foo"))
	require.NoError(t, err)
	signatureA := &Signature{s: &sig}
	signatureB, ok := signatureA.Copy().(*Signature)
	require.Equal(t, true, ok)

	assert.NotEqual(t, signatureA, signatureB)
	assert.NotEqual(t, signatureA.s, signatureB.s)
	assert.DeepEqual(t, signatureA, signatureB)

	sig, err = key.w.Sign([]byte("bar"))
	require.NoError(t, err)
	signatureA.s = &sig
	assert.DeepNotEqual(t, signatureA, signatureB)
}

func ezDecode(t *testing.T, s string) []byte {
	v, err := hexutil.Decode(s)
	require.NoError(t, err)
	return v
}
