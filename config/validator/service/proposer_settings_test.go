package validator_service_config

import (
	"testing"

	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/validator"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
)

func Test_Proposer_Setting_Cloning(t *testing.T) {
	key1hex := "0x94ef47878aea6c24a6aac5d43465cc361bbaf8bc8c9eba9abccda48977767f5604b8150337fd5ca7cf90bf8f63fca0e6fc0728a3071e5ccae2766a15679d2a57ddc95f6f51ff8bb831aaa937271eb80d599566ae1e73173aad708f68330bbd9c6555c0f9366763011f7aa4edebab101f99a4007c8ae1123a13e7c7cc19e2e7699b549bb770d3753bea49ec9e31104bae89fc38abe75e1f140267a2f492409d25f188aec4783afd2c8140f6a8c6850077536cb2760c65779d165b6f03f9b149210d8160f58803d31171be717baf887aa612c02f806bd9e3332ddc21c0e6e912d053d4d49c13d2de8a75266e6157610175d4897e58886aae12bf7b949e20351d80a4a994e7c70c9ba76a2472818343609061ffa393f6f270dc8b4aa806d5616e55e936f26cccd3e1006bf185196ae5457d224fca6555068bfb64a228e8b44b4221e85d2f1137224992f41a78301ff768527e953e50424a45d21e8877a16a915629f45aba1aca08769c561260d4d58bcf36af98c26e6b81365b91720f3155c1f9383d8c7ab295aabe6c9799f625d29da42bd4002fbd337ceaf991573678f6384da18541c4e3a3c9472620c19129bb41e2b5e71884b98a8ac319a0ec2be11948f4c02b0b824a2347e00945ec89f45f431dfb3605d8228ce69136867570bd0149f4fe2b53f19d1458e7d6f9688af7c5ec3021fbe61cb331a5a44c9f5e9a4a1192d5eeddf98f47e1c8379bf000ae6886cc3eb5442fa8586652550876a4ef31dd941eac082e9fb1ff26db706159ec7be0a5051b408fc955c55335db1f46c6e87113aabb03960f2f4fc986e4e583021b6b69e7c68b0d1093429630cf7f4a7e895ad45a41363c53d0ddce0804a8858092bc7a069852fff02773e0abd6c7cc7d3d34c0bebfb34e1c5e95ddb184c4a0fc77ed09fca96dc472ba0391214c489890ea410085d4c6ecc69f3facf0b1587372752a421641597563fedc9fc64452d7a7a0db9560ec1c8564a3180a5e623b65e1dd494f967556bde56f9bf58cf5e07a050258e4a90cc2700831a4113391877c65ebaadea8710e23ca9f7afb8c5bd90edf38211b32874b65bef5455159c1a1d17ac3da17819f8adcb254a62d7f4c362cc470f75fddaf6f6624f3a56b0187c4e21295534a832a0f2720c411d0685751fff095078d18bc854856f7e1abe14eb76e9e45fd1eca282804784d9c27fd15ab00bb6dbfd864c401f759d0f2da8ad8640b81066038b72c6cf26605f8388cdd67aeca21dda79d6dd01bc3d3ff5ee29f5e016ea681ba581940d0130685d42e9635c6ca27e1eb9fbf08f44879f4f479eefbde7b65476fbe379f771b0f116a2e6e5f65416b72f2c49452c40b2f108ab86ac7dbb8e252ed32946e5be280b512734c96db9511b21eeec3d0caeac51f8ca315ad7dd62efd1113e03932856d5eef73035a0ae24fef22a2c8aa1db28bc87702c34b2d2d722ddf0ad9d2eb4a6c16b85a7e22c49d115c676afeef8f66ec95ec805e4c1423df5dc2eeece55f107005e15b3ded7c5ff7b7d6fd8530049ede7b776bee594fc18e29a4ae177419bfcb2a0185f51ab35c389baf9b37742192e1c36c2ba7d5c46f683dfa8ca8103a824033c68d844d0ed55e560b96b8421162f57e2daaef5853bfe476903fcb42196e78ae2afe3d5da230de9628e2d268dfc50290b2a8ddae0ade58e2919e0f9be4b538220b885e20b28741cce7c6072aa27cf076197d8e046072045f4a4e20058e7ff428419ce7629b7f76dc0568d99586fc4095107b02228d4f2e9978abe68ef302747a67805d0158b88e94bdb73f1c6e1ca8918b311b0a5d101130e142d2085778d1546532c842f2673cb7a3774eb6d23fa901883aaa682aa7ed0195c3f899f6b485e09715a3791ac9102c177b37465344523fef3e9479ce39d358db0106b5b4cfe26415c5ebc2d00ab36af1171ca33f2c95ff16b63f91e10de4405aec7d6368afc20643616bf30505507f7af84a6f70a36c9f6644bf22bb141f88f15c01f99fbb0344b6db03c042cb8e80e6e38717c3e749cae3782b3d3529cbcfd68a04ade376a7f334ea471d21ebbc62035a57cced3f74a1612edb492a13aacc93d51348249451616f01bbd0e89f46fa53acc5490c7164ef8b2ac0a236b9da37f696db7d2e2dd51243966d2deeae418f2edc6f38fabe746ec5bc832db5af6856266c140e78ce15c2699778910c8e002290b52a68bd14c3be294154c7f448be0160d4adc856b307aa4d3a3ec82af0d25d951d25dc2027ed7861c9ef7d0228179a9fd38ba45f8721d6ce3dd42dce8f58f917d3555c04151cae3e0fc761291c632c0f13e618958cda614f7650efd18e1ffed06bc171530b5bde901becb2b021db47e541679bbb55f7337e1d205e1031eb2ef9a332e84bcbd9b5e27682159c86d3031ab01d741fedf1b05a4e1bc82da108855c8d833abb99821c8be81df68818e2aa094a3cde6f3d5e1bd8b2e86daba12aa2b572ffc81c65c3e498432edb00f1fe6fb04ec92a96b2c206a36a5623f8710c06cc20fdc661230f8b441ce4ebfa45a2890a4a43f2dbd498a9ef9d1f4e748ec81bca42a27aa8acd72406ca303050a32aa644f60e1c58a036ca2b0f0ca69092f6d08a40ee97ff700931b87039bdff71043a75b1578b33d98b391ae0dccbd46f5428cc80016412cbce9532b70454ab801c77072249412a2d49e8f608ab7480b1a9416714c825bd07a96641b865daeda71a5bf6b9e28bbf4a9042e79a6d6dd0c1d99dd3cabf4d580b6bc22999acbfb6f25a33e622104c13aa173e2191eb70dd0db82ae47d1ddbbdb24d3d4403bcaa64bcde88c86ce5bd535694b24f117e729abb3582e2166658f969a206b44aa37837c6efd7f094443c65f43b95826aa97ab1d3dc9aa3e56b31b1d2fe5e2eb44d2b5ccb1039118ac3917148fe4dffdc81daac74007ce71dd5e779be416de62b271bc4379e0a24c6c42e8e8c0213dee8588752d54b12b4da7be2e7a75c6b3e8ad9a92a4768d0611bc91ddd4aebda0cfb84226280ce6f621b5a83016b51bc9de2fe0413ce43ff967cf3680c9e1c359316ab207d93382df330b6a1fff25f01506952465fbdc1d36aeb0124b593b29619b712867c63c7e872d65d18b8834c505ff23688bda7b9d4e4969d6b69aae0b5ff14a152191c5f94a061aa3a7db71bc4ad9ce217a931a92b35e6faec8e00800f96b0efe6d42d1edf25573f12da245539f8fba9ab270cf738d585a144d9098e5d529f3e8662903de413ca9174b9bc6da0a70be23cce8b1dd7b0a1db109605f20d3ba5d72d4361e63cdbfe58ba1e19c0cbb0ab65090c8dc30265ac76a191707804756107d14e1ec41b6b8765286f99960fb601394bb1db089bd5ea19f3b98666e003affb6e9477a42f1c836efdb2355ce392849a777a6c4ac9e1eeb7bad57faa0f25eec2adf2d2d3b20e5ad1ce82ffdd9264e90a37a269f24841742479ccfadb38664c503191da287b16ca59d06b0ebb09e658cc0090665d8ca90b917c4a089b9474b9d76ba7d9deeb96f9a82ed10365d756da05f23bc9f81222b09e5d4b490977052f0bdd3afacd28b2f6730b3a6784eff8653fc8dcca5d17f7c34b05a30cf939ca10c52b714a5ab51b77b523917963a9f3374c7004b81588e06103615fb793dc267e8e3677d8fed75cb371"
	key1, err := hexutil.Decode(key1hex)
	require.NoError(t, err)
	settings := &ProposerSettings{
		ProposeConfig: map[[field_params.DilithiumPubkeyLength]byte]*ProposerOption{
			bytesutil.ToBytes2592(key1): {
				FeeRecipientConfig: &FeeRecipientConfig{
					FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
				},
				BuilderConfig: &BuilderConfig{
					Enabled:  true,
					GasLimit: validator.Uint64(40000000),
					Relays:   []string{"https://example-relay.com"},
				},
			},
		},
		DefaultConfig: &ProposerOption{
			FeeRecipientConfig: &FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
			},
			BuilderConfig: &BuilderConfig{
				Enabled:  false,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				Relays:   []string{"https://example-relay.com"},
			},
		},
	}
	t.Run("Happy Path Cloning", func(t *testing.T) {
		clone := settings.Clone()
		require.DeepEqual(t, settings, clone)
		option, ok := settings.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, true, ok)
		newFeeRecipient := "0x44455530FCE8a85ec7055A5F8b2bE214B3DaeFd3"
		option.FeeRecipientConfig.FeeRecipient = common.HexToAddress(newFeeRecipient)
		coption, k := clone.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, true, k)
		require.NotEqual(t, option.FeeRecipientConfig.FeeRecipient.Hex(), coption.FeeRecipientConfig.FeeRecipient.Hex())
		require.Equal(t, "0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3", coption.FeeRecipientConfig.FeeRecipient.Hex())
	})
	t.Run("Happy Path Cloning Builder config", func(t *testing.T) {
		clone := settings.DefaultConfig.BuilderConfig.Clone()
		require.DeepEqual(t, settings.DefaultConfig.BuilderConfig, clone)
		settings.DefaultConfig.BuilderConfig.GasLimit = 1
		require.NotEqual(t, settings.DefaultConfig.BuilderConfig.GasLimit, clone.GasLimit)
	})

	t.Run("Happy Path ToBuilderConfig", func(t *testing.T) {
		clone := settings.DefaultConfig.BuilderConfig.Clone()
		config := ToBuilderConfig(clone.ToPayload())
		require.DeepEqual(t, config.Relays, clone.Relays)
		require.Equal(t, config.Enabled, clone.Enabled)
		require.Equal(t, config.GasLimit, clone.GasLimit)
	})
	t.Run("To Payload and ToSettings", func(t *testing.T) {
		payload := settings.ToPayload()
		option, ok := settings.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, true, ok)
		fee := option.FeeRecipientConfig.FeeRecipient.Hex()
		potion, pok := payload.ProposerConfig[key1hex]
		require.Equal(t, true, pok)
		require.Equal(t, option.FeeRecipientConfig.FeeRecipient.Hex(), potion.FeeRecipient)
		require.Equal(t, settings.DefaultConfig.FeeRecipientConfig.FeeRecipient.Hex(), payload.DefaultConfig.FeeRecipient)
		require.Equal(t, settings.DefaultConfig.BuilderConfig.Enabled, payload.DefaultConfig.Builder.Enabled)
		potion.FeeRecipient = ""
		newSettings, err := ToSettings(payload)
		require.NoError(t, err)

		// when converting to settings if a fee recipient is empty string then it will be skipped
		noption, ok := newSettings.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, false, ok)
		require.Equal(t, true, noption == nil)
		require.DeepEqual(t, newSettings.DefaultConfig, settings.DefaultConfig)

		// if fee recipient is set it will not skip
		potion.FeeRecipient = fee
		newSettings, err = ToSettings(payload)
		require.NoError(t, err)
		noption, ok = newSettings.ProposeConfig[bytesutil.ToBytes2592(key1)]
		require.Equal(t, true, ok)
		require.Equal(t, option.FeeRecipientConfig.FeeRecipient.Hex(), noption.FeeRecipientConfig.FeeRecipient.Hex())
		require.Equal(t, option.BuilderConfig.GasLimit, option.BuilderConfig.GasLimit)
		require.Equal(t, option.BuilderConfig.Enabled, option.BuilderConfig.Enabled)

	})
}

func TestProposerSettings_ShouldBeSaved(t *testing.T) {
	key1hex := "0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a"
	key1, err := hexutil.Decode(key1hex)
	require.NoError(t, err)
	type fields struct {
		ProposeConfig map[[field_params.DilithiumPubkeyLength]byte]*ProposerOption
		DefaultConfig *ProposerOption
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "Should be saved, proposeconfig populated and no default config",
			fields: fields{
				ProposeConfig: map[[field_params.DilithiumPubkeyLength]byte]*ProposerOption{
					bytesutil.ToBytes2592(key1): {
						FeeRecipientConfig: &FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
						},
						BuilderConfig: &BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
							Relays:   []string{"https://example-relay.com"},
						},
					},
				},
				DefaultConfig: nil,
			},
			want: true,
		},
		{
			name: "Should be saved, default populated and no proposeconfig ",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: &ProposerOption{
					FeeRecipientConfig: &FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: true,
		},
		{
			name: "Should be saved, all populated",
			fields: fields{
				ProposeConfig: map[[field_params.DilithiumPubkeyLength]byte]*ProposerOption{
					bytesutil.ToBytes2592(key1): {
						FeeRecipientConfig: &FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
						},
						BuilderConfig: &BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
							Relays:   []string{"https://example-relay.com"},
						},
					},
				},
				DefaultConfig: &ProposerOption{
					FeeRecipientConfig: &FeeRecipientConfig{
						FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
					},
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: true,
		},

		{
			name: "Should not be saved, proposeconfig not populated and default not populated",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: nil,
			},
			want: false,
		},
		{
			name: "Should not be saved, builder data only",
			fields: fields{
				ProposeConfig: nil,
				DefaultConfig: &ProposerOption{
					BuilderConfig: &BuilderConfig{
						Enabled:  true,
						GasLimit: validator.Uint64(40000000),
						Relays:   []string{"https://example-relay.com"},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &ProposerSettings{
				ProposeConfig: tt.fields.ProposeConfig,
				DefaultConfig: tt.fields.DefaultConfig,
			}
			if got := settings.ShouldBeSaved(); got != tt.want {
				t.Errorf("ShouldBeSaved() = %v, want %v", got, tt.want)
			}
		})
	}
}
