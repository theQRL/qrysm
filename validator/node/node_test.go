package node

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/cmd/validator/flags"
	"github.com/theQRL/qrysm/v4/config/params"
	validatorserviceconfig "github.com/theQRL/qrysm/v4/config/validator/service"
	"github.com/theQRL/qrysm/v4/consensus-types/validator"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/validator/accounts"
	"github.com/theQRL/qrysm/v4/validator/db/iface"
	dbTest "github.com/theQRL/qrysm/v4/validator/db/testing"
	"github.com/theQRL/qrysm/v4/validator/keymanager"
	"github.com/urfave/cli/v2"
)

// Test that the sharding node can build with default flag values.
func TestNode_Builds(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String("datadir", t.TempDir()+"/datadir", "the node data directory")
	dir := t.TempDir() + "/walletpath"
	passwordDir := t.TempDir() + "/password"
	require.NoError(t, os.MkdirAll(passwordDir, os.ModePerm))
	passwordFile := filepath.Join(passwordDir, "password.txt")
	walletPassword := "$$Passw0rdz2$$"
	require.NoError(t, os.WriteFile(
		passwordFile,
		[]byte(walletPassword),
		os.ModePerm,
	))
	set.String("wallet-dir", dir, "path to wallet")
	set.String("wallet-password-file", passwordFile, "path to wallet password")
	set.String("keymanager-kind", "imported", "keymanager kind")
	set.String("verbosity", "debug", "log verbosity")
	require.NoError(t, set.Set(flags.WalletPasswordFileFlag.Name, passwordFile))
	ctx := cli.NewContext(&app, set, nil)
	opts := []accounts.Option{
		accounts.WithWalletDir(dir),
		accounts.WithKeymanagerType(keymanager.Local),
		accounts.WithWalletPassword(walletPassword),
		// accounts.WithSkipMnemonicConfirm(true),
	}
	acc, err := accounts.NewCLIManager(opts...)
	require.NoError(t, err)
	_, err = acc.WalletCreate(ctx.Context)
	require.NoError(t, err)

	valClient, err := NewValidatorClient(ctx)
	require.NoError(t, err, "Failed to create ValidatorClient")
	err = valClient.db.Close()
	require.NoError(t, err)
}

// TestClearDB tests clearing the database
func TestClearDB(t *testing.T) {
	hook := logtest.NewGlobal()
	tmp := filepath.Join(t.TempDir(), "datadirtest")
	require.NoError(t, clearDB(context.Background(), tmp, true))
	require.LogsContain(t, hook, "Removing database")
}

/*
// TestWeb3SignerConfig tests the web3 signer config returns the correct values.
func TestWeb3SignerConfig(t *testing.T) {
	pubkey1decoded, err := hexutil.Decode("0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c")
	require.NoError(t, err)
	bytepubkey1 := bytesutil.ToBytes2592(pubkey1decoded)

	pubkey2decoded, err := hexutil.Decode("0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b")
	require.NoError(t, err)
	bytepubkey2 := bytesutil.ToBytes2592(pubkey2decoded)

	type args struct {
		baseURL          string
		publicKeysOrURLs []string
	}
	tests := []struct {
		name       string
		args       *args
		want       *remoteweb3signer.SetupConfig
		wantErrMsg string
	}{
		{
			name: "happy path with public keys",
			args: &args{
				baseURL: "http://localhost:8545",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b"},
			},
			want: &remoteweb3signer.SetupConfig{
				BaseEndpoint:          "http://localhost:8545",
				GenesisValidatorsRoot: nil,
				PublicKeysURL:         "",
				ProvidedPublicKeys: [][dilithium.CryptoPublicKeyBytes]byte{
					bytepubkey1,
					bytepubkey2,
				},
			},
		},
		{
			name: "happy path with external url",
			args: &args{
				baseURL:          "http://localhost:8545",
				publicKeysOrURLs: []string{"http://localhost:8545/api/v1/eth2/publicKeys"},
			},
			want: &remoteweb3signer.SetupConfig{
				BaseEndpoint:          "http://localhost:8545",
				GenesisValidatorsRoot: nil,
				PublicKeysURL:         "http://localhost:8545/api/v1/eth2/publicKeys",
				ProvidedPublicKeys:    nil,
			},
		},
		{
			name: "Bad base URL",
			args: &args{
				baseURL: "0xa99a76ed7796f7be22d5b7e85deeb7c5677e88,",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b"},
			},
			want:       nil,
			wantErrMsg: "web3signer url 0xa99a76ed7796f7be22d5b7e85deeb7c5677e88, is invalid: parse \"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88,\": invalid URI for request",
		},
		{
			name: "Bad publicKeys",
			args: &args{
				baseURL: "http://localhost:8545",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b"},
			},
			want:       nil,
			wantErrMsg: "could not decode public key for web3signer: 0xa99a76ed7796f7be22c: hex string of odd length",
		},
		{
			name: "Bad publicKeysURL",
			args: &args{
				baseURL:          "http://localhost:8545",
				publicKeysOrURLs: []string{"localhost"},
			},
			want:       nil,
			wantErrMsg: "could not decode public key for web3signer: localhost: hex string without 0x prefix",
		},
		{
			name: "Base URL missing scheme or host",
			args: &args{
				baseURL:          "localhost:8545",
				publicKeysOrURLs: []string{"localhost"},
			},
			want:       nil,
			wantErrMsg: "web3signer url must be in the format of http(s)://host:port url used: localhost:8545",
		},
		{
			name: "Public Keys URL missing scheme or host",
			args: &args{
				baseURL:          "http://localhost:8545",
				publicKeysOrURLs: []string{"localhost:8545"},
			},
			want:       nil,
			wantErrMsg: "could not decode public key for web3signer: localhost:8545: hex string without 0x prefix",
		},
		{
			name: "incorrect amount of flag calls used with url",
			args: &args{
				baseURL: "http://localhost:8545",
				publicKeysOrURLs: []string{"0xa99a76ed7796f7be22d5b7e85deeb7c5677e88e511e0b337618f8c4eb61349b4bf2d153f649f7b53359fe8b94a38e44c," +
					"0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b", "http://localhost:8545/api/v1/eth2/publicKeys"},
			},
			want:       nil,
			wantErrMsg: "could not decode public key for web3signer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet(tt.name, 0)
			set.String("validators-external-signer-url", tt.args.baseURL, "baseUrl")
			c := &cli.StringSliceFlag{
				Name: "validators-external-signer-public-keys",
			}
			err := c.Apply(set)
			require.NoError(t, err)
			require.NoError(t, set.Set(flags.Web3SignerURLFlag.Name, tt.args.baseURL))
			for _, key := range tt.args.publicKeysOrURLs {
				require.NoError(t, set.Set(flags.Web3SignerPublicValidatorKeysFlag.Name, key))
			}
			cliCtx := cli.NewContext(&app, set, nil)
			got, err := Web3SignerConfig(cliCtx)
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, tt.wantErrMsg, err)
				return
			}
			require.DeepEqual(t, tt.want, got)
		})
	}
}
*/

func TestProposerSettings(t *testing.T) {
	hook := logtest.NewGlobal()

	type proposerSettingsFlag struct {
		dir        string
		url        string
		defaultfee string
		defaultgas string
	}

	type args struct {
		proposerSettingsFlagValues *proposerSettingsFlag
	}
	tests := []struct {
		name                         string
		args                         args
		want                         func() *validatorserviceconfig.ProposerSettings
		urlResponse                  string
		wantErr                      string
		wantLog                      string
		withdb                       func(db iface.ValidatorDB) error
		validatorRegistrationEnabled bool
	}{
		{
			name: "Happy Path default only proposer settings file with builder settings,",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/default-only-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
		},
		{
			name: "Happy Path Config file File, bad checksum",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-badchecksum.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0xb14972bf2d2bf174f40a11e534fc300595e0a00d21260f939442c7feb7015a32d93c98599317e264909d643a547d785573f68d121f25d768db8233e474a8b793bf54247778bd303ab27ced26b2cde9ea125c208a62dec187934d456880f8edd8db0f6b393b5fb7cc72324651e4f0154afb067ef203e2641d5bac67f00b7cbc8278e443ae5851fa17d555f8e859c10d91c0e700c4494bbcaf353ed1e548040ac89027a1222a180e1022c1292b3b8264ac292fe6f59bc08e6b286a4bb490da0905e33955998ec8c554dffe47f122d41a37e82e0f1aea711b5504d91979ebd6ee55ad9d2406eebf727e3ce4e599195522cbeb62d9b6c3ec251be38c7e805c2a3edd05fdf2e3c0afefde0a4195b898b988ec1c5d2cf4b66314553e49605c99b830f1917f5dfd2258aab3e512b39fe802b1b86d0a7f4e56ada7d76d954d929e76db07b2a8664d6f6cd484544fb1848aec9e21b813367e203d92f709a111726a025685ff15cc02b2e64f12a5a1c21a34e699d859a7974999e3d33621c69aba41fa81cc028e6cfb025e3671864d88a0b11d3c97098523ea41113b046884ea7ea708703854941d84b9d8649f1a9dc0478efc250eae3a81c7246df63207d177908227e0e0f88b25639a685526a7f96a034f1c91ac37c94274dbdb08bc6ed81501cc74efa38f7a92264f5afc9dbc57d2dc54d676203274fcfe9dd3a065fe2210d3c23727d91037329b13785bd8e12aabd4a3db5b38d9a979925411bd33e1b3c6e66e1e142a2f6d4cc310c84de6f2d337f21256e20c66aa20966662cc7d07a71d6c523c8242f98d03a1438efacf1261d2c76cd77d6823255188708b40701d4d15a0e16e5274cb06c2655464606b2c13168485c5c9f6799ea53cd1bb320958e5685bad985e840e37d384ef977e591e2870a9251ad799fdc6b5f1fefd1b8415905f391926752df43d9ed0256bfbab08ab471a52fa3f14c72f123bc75b062f8c5242dd0068f0f950c737f8e24bcdf5439aecfc34eaeb40d56bb685c7809ec2f83bdea0c5f06b7ea7ea49b30c2b3fc938803c176f323ecab7cdab68338800a3005a52b751fff44cc463b86f2cc9a0f0b1dc6c53d543c3488d5ca895e00dacdc3dc9538d4444786970969ef0d123c66982b97cd0e701901157f35c73cdca13223f7db046821ddfdeca23d4a135a9378c7eb45a7bec26a67ca6f4af118d55c0332d2a81a054d22b52e139ca6034e58c39b8001e20ea053177d234ae96ba76423c91e4b6b6490dbfd1cc3cc1c83238e80e4d8ed422b480fc8aca2c6c4cae7413c6b701f41e94a04a04a153e2cc89f139f44d7f146e778d7911c8dc01ee72eb7a4636c74bcc0d04bc43d73da8a53939e0786bec4959e2f29fa94cda57a817fe5fa6dc0113a0af55d2c1feca679b1447ede000a17322d9d71a5e320a4a8ecc366f6a28b790486bfe663f7046ae9f8ac7d08944598b5e332488748aef4b9402ea0f03e06f3976c54b160efdc97fbad1f70dbdd94a31e758edfa01f7d5169a1f231d3f169b700f4526fa7e9eee4e70b112f70a39b3708d73c6959ad7e3c15acde57a6da148472a5ad51a1a5c4b393c6a57728acf22c6bf90dbe4b9cbafd93f0489d14173dad2a8e5ba3aff6d0fc2d31d3f6be46e663f8438936b25a322804a54d30350ef2eef9c19c4cf4be3dbd4726be7932a585e8d7b998d28089bc68682c75aeae6a9e0c5715c890b69baf33a18ee15f11605b10dacb6bd52957b5eefbf17f7f82d6f3e783c7a1e793d0a5de8dd5aaecd37c46ee8b4f709de97685772f0295bbb527d62501940cba437866a0e6e178bbb7eff65cca6df6992d1f22f3b22ce954c4208593a8fc27fea30d6bfa768db2bff3547b13d3fd16002a1972b0d4b48b1f96860fcb73f2db64c1ec3c0ee5d76ba7c02c88fa6c400de822fc5ef81d8a3093f283684a15fe11f9236ecb57c5804b03c0fb75a9ccdc368b55ebbabe75bf5787280fef5c82758fe8337b2c201cd825916494da6f8c74b2ac8d1e7a2543d74ac36093a273913c3a1748bf7200ac39037f683f0dfe61312dc2eb3fe4bf7258b8f957606dc54c420cd9e331b2dbd8e7a7d8aa10e167dafd0f92a789b5c47d8629751c288ea9da7e0159c5bf9f9be82539f00818fb9eeedb53dde5e52a1d514a659020efd5defc7d1065bb08d6aeff11163ffb2145e972dd5907bc4abee2a26765923fc1aa49a5500e05eb8822338c5685d91f7d969616267d81f590f501932cb6d14e71a8d81e4f33e657f0da0c434fe11f9207f86442bba681d9d1bdc2d6fc6f6e0ff5a8b1db0e8bb81d605c87143072635d4d0429334608badf866945302d75e515e3b8b80e69b3c8aae2b5318c7e1ed543520a6a3d02940959c541a2ebda088851c80594aec2dfb369b0b7d55170ed705ae2281aa2d48a5042d07c3e76a02332c75ebcc34e48eadddfaf17bdd5ee8c47b04bc38bce277b834215e9cb6b46f9f305ae14a8bb54ba399f0352baa592dfd1af5e3ddb8bb7fac979788183774dd8b508ef41e9bc659f13892fe1df943dc476e0c7b224d08464bd8f6e48a189d1037417068600ade3f8368372077806c1df66448a4ada2ddb244eda880489a3f9baa842d587bb625c98d209f3ad53935aad522d7ab6645356b04223ed745aeac6a860e0dc957552d2275fae5be96977588fc1bcff98c2d636996db9b8618012781fafd8a4f55fc692040eb2d8f867b1b945833aace8360efdf7499a1039d77bdf4216856df985e2ce60488aa5182060ca16fcf3da239e7c3961fc098bece56818b1d0cd230dfa7a942837c515e792f200c90a0eab544e94b4f514e5a18adfb03199dda5b3dabaf1e8410e1b5c16028886170c269429239e42a62165e0a69619378bfe0e952f395768906b2e4e8e136804a82a87f3ae4fcf8c374cc701b3552a3581e8fb4c43fa565e94db33ac95e52479c011c89d2ff6f976222b0497f64edf817b49226efd99d712aab9e09fb28023c3228669283ad95239a34697f4a6cae1480db4372c2a707433bd737a57b6a9ea8deb94a53204d474960f89ede7c1ffb7bc8cd8abf702a3ae064db0829f13c5d0166821d891e0495b08889e7a80bed1d89b60a14c3d7ed895c72f6056a193a7a9ed1af26a275dd6bfda731620de376d610bc53dea6b12db3a49227a2fb9696a9e96af7800896f2f5aaee2b1399a5ba3c22f69cb7b6d25c5afc29a24d523449c13b6f492a80c8017db51f54fff69a35938661affb226df15626d42722be938e61bef7e75e8bb2980406b76565e1f5f5cecb927eea5448136ab1cc0e653ff54fde07eeeb78e8de783da19629fbd1469f57183238f2c82ffa18598113419333c908ffb90c6a3fa4c6d0afa0c370447a9e350ec115d6347efd26d2ab2fbd726dbb788636a23b59ef295f472ba36692b5282b35d3e1a1bf8d202825a7f39b610e1965576d78daeb6a122927ecfb632ca2af2d0ff36928e100ff6e274aaaf69d287834128506ba3dac7a291228cd8e82ca2924d950d1e2c1ad5e998a24ff377554d14d64d25f4e9d7f0f57608989ea877ec9c2b19561e50390b5e7106ef418aba476139fc1a822e7cb53a03c39b4495c767bc2197060594785f0c97704d281344c14f550e70f8a15ffd0eb32bad77d15693e8b08b128c3ad28105a40add155da")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0xae967917c465db8578ca9024c205720b1a3651A9"),
						},
					},
				}
			},
			wantErr: "",
			wantLog: "is not a checksum Zond address",
		},
		{
			name: "Happy Path Config file File multiple fee recipients",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-multiple.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0xe3606c3ee8f8b8fa685dc7e088323c51e8def62e1e1d479381d78c37457c12f68f3f9cc0404b5dc2f6f03a812828088159f47aad446c44931e02a010c91eaffa487112ba02157227ee49495b91e4d077a9f1fd90fce4f7a61af33ef7929c338d8367bd8e7f0608528b04156d340f659ed49f7755236fcced390ddf5a2b74ecd9ec41c786315b2a400898daa1c451af88ef3ce07d0852f39dc95ae38ba86f0670c14cdf67c844846621629bc92e86ede8035340d50a4957692de478311f82692fcbd90658f17662d6c82357a2f1b70785b5cfd64a3c74c04cead22b32b831a71055a8c6bcf1eee6b45912cb90257cba6618e2a33a0bf2309b831c69d0eedf4b52ddf4c5af2516dd0fc094d8d250ede7a15cebfc33d517dab62e6ab98938eeb0913fb582d329f480ba016ec50d249bdd31606b17230820c01786a9709c62c62bb5d86727d1a0a7ae347e135b5bca49e6fe3eee6c74623c5d1978dee0daeadac5afb07511e4a0771c216dc81f4f30340afcf68f158783becbe55144781e79515baee62b3e4328462770df469c4a165f6127c24f7945a833d84bba4b962f823f02739e50aaaa11dc53674f77a76df3045f7fb867c2c396723358a801db5f758f82b4a38d3cd60e5135b23a37d5f5f88ad77eaf61d2500661768241d0f6064a34905e88a15519ea1b2090c61e56d7bfab0156bcc6ea407a04a361d2068ed0ddfc57e0cf79a4190d4b6abf254b711b9c72014db2316c0244c89e691b57c490013e47e0bda3649337eac3b7a3c7931b973de0e3e8cd514c1c2b243b18838e69819247e20cc445e3ed07283c9ff32e5d62ff8e7cc1284eacadb05673a082d318eea48271e43f6a4c1081003186c57b86ec4f6cdd42d6275373f2c27203f8e0ea39cc95798a51a8c540331e74210552917ff08b5601ce921d30949e7e2d8b6a5c62721d6ccafd713632256b36575b4ea38c367fbda360f6ad7a7f448d7b152cc7679921e7060f73a331b24239ca01e2c0c098676f7ce62409d3a5af9c32eeeb4f77fc832775fa542c3e8c329f7a5319ec223d13a8d83d72f85fe712e7cfc0a61adbb26579e774ebbcb4913efeec7cc980c3a41d026cf26a50384ccdbbbdb79e4b738becf03146318e7f84b1927c5df44cdca51fe2ea82f83f3dca112e86cee9267f277172a837a6d1d787bc915c5353f615c425534456d640b86b33f1f70b3b87cd2372b5031e0646570341bc1c2bc806b46406976b3e8a9087de80b22396dbc0878c2243ed2870354d5497490c32ab3fde9f4fc01732f7c5f8949167582a26ed2ac05168b30f7660f54a4eeb529cf68ee57877bf8c2e846c5b9a9dd6917f2cb1808524f285f554cfb2da4020bf9a92ddd4c4ed0f7d6699255ed3429297d0bd7f1489c95cdc58411d4e08142e15e33a0434a172286cab683550b7ea35a5c9d377d6d657ec133944963dfbce0dc8d10b9c8bd277cc7513f82f1656c6a0cee2e5add2f550d96f9ad57ce7d20c7658ed274d6b7d1cde88f2ef2bbea20f1b006f2cd85115734e0b59429eb3b9f1d77ae386e80c4d384007fb30fbfbcbaa3f704d8d989494a4dfd59acd9f6299d82dc858bdad5d3be28d393152709b49dd3000aa15e1fdbce58baba08d8e34173444bd47eb8def7baa7a5a3698427e00e4be69840d9912e903bfa7dd15b1e3aca77bb2338752a9d176c98c0901d0222fa66850c5b465950b477b271e7cbb943a3800018361258fdccdc5d18da166b89a8394be8774cb42bd7fb794299b4519bae756b4ccf5b3c5113425860fca75d1d2443e3ab36d7536f2a2b078b3f78f264579a57478915d8a69745dc1f6b8b11611b1f5a30d494d12c2343b13f3b037c9123eae3f9534ca04e0214403de4c26d6c7009beacbae6750a97fca4c798ce5f6329657ae3cdf2f216994cbe7bb20b0f4d56ed9da86fcaf8654362fa4653451f09cc71b86855355ae56665c865722639a69c1082415450a3d4fb386879340abb5b9369681f675b62e806b79807e006d4c40af9a80a974a266724b68eeda61461929c578d4ad7c82b2d46f00283ce73a9ef27820c5b8465446677a05a5630ebdb3bc15873f2b7ef2d2122b1a32857d1cc312afa2b7868e5d16c5d73efa6030bff49d3e461cc3a75ee46ba61dc2ddab8196045ed11d4ea3671ef1e1672b2fd126e1c4046150f5ed16052cd4f2ec379d132c4f889013e4cb444e16b1c77560f9911937dfe2d36bba0c2de8cf2f80fac97d77a1f1d76fa003f1c5200fd3dd6af22459ac9f23202e4817b58c1187a2176474a3cb5fb2d55d660055122cf9f207ca78864280f7b961809e84fd59472aada2a1925742f3807033e684e9fd0ce469d982dd03c66cd08970a7fd59652d23a5ff060873699258a658fd2452facdb320e23abf8684b47180292c8a7c4b2a6abdbef2e25b265f493c624f7c6ee2950342ff1ec10c5b091c58b794816ea0254c12f27c3b00602c1e3ba90fd3562e5842a755e1ebb7ba28a988fe1e3c8622867be1bc862892c432d401231b5c018da101e2aa3bca5e638463fe5ab7f0d3a420533dba2777b0fae52f8ed0b12f856ca3695c6bfc0b2b0a8347452ab0e07904ec6ffde7fa1d1f661eada6fa6b79a79304c2879a6bd6271acea1732545003811c64db103cc4a75ef2b748d79666c01fd09960aa9e1c4724557ec5c42fbb508af25e0195d7075b37709234ca311de6ca71062de27ae7f74106e1bd63c08081db421684de9d31bc95ecf115a4b645dfe4550800792a57067b0ae25c7eef56e3443dafa424c590f261c2e8c292e371c1e375db671d8bf7254db80a12dfd3c68ddd9202466774589776641994c8e41576229d6a39d9e6929f24526916d2008a7ded6a7a710c1277fdc297cf223ff10ae92cb06633a063a667a0facebc8c5644652c112792d781eeb5f2c9116871f7d77414220eebe3e34adcf13d46d31bbdd8d2ca302c62a335cbcdfc7a6fd16c1fd33e2cab1917d33706e42501ae85a7cc031ffa4b38d1b30ce30cd862c31c742d63a50bf4506753fd88feed05f3ed3cf3ae3b7de120105aca07937086c2aa81ffd8a2acd76c2282b4f87c9264e53a144ac5794304500f99eb596652739ad8b936d1e7882706c330e144e69740821d2d8059f9b4e523f14ed9d1a7e01fdc06074837a89f53251da35500ccc1a77a7df081d434f81227b99151a485b5abcfa5b3400449122622ff26786c60d0bd0db11102021a6ac624bd034625552dafb7984c5827b14ff6a152677d6efd0699703fcfed6ce9e59b0b7a5d19594bb7398c18ff9552ab7ffe254bcb628a4470acdb46c785e06d6d12c433be513df2e3bb9926a5ac3a2964ee93bbd8e7987df60d8a952d30bde7d571c8a708659b0e6f3b9880a209146c63c8f64cacc66ea0f4d775da0d088437863fcf56a580b38b04e95fd91e8aa92a41cc56338046247051beee61dcb255bb5aea7203a62441296a7ff9078f44b3726de86cd3eea9016e240b68fb49eb63629d1c9dfc0d425516bdee339bd166ee67a8bc910abb88342982b27931ed5af1517db9fe1492b4a59e6065a0579ee2accd3ee210d5387b64673e70a4ccae434f30318f4924464c42e9c71f0ddb68bbca5e7e45789e50d8c4809f1e52b0d33c4936dd7d884e0d45794adab1d327e86610acefd1b")
				require.NoError(t, err)
				key2, err := hexutil.Decode("0x996a1c414f0f7b66681a070803d9aa4e45e1e0cca50bff99d1d1b80874cb50a6c50446275c8ec7bb4a0c76470f789b03dccc291f2fa7c4fc2a19ba4583ec31b34c960f8c71f3b2ff1ff156e3bcf49cf9b92b914ae60924920c2adeb52940939ea724ac125340fdedf85d6d675793a9a835559444ddd423faa05cbfc4dbc4c3878dcd956a9ec6ec7c56ff6e54e7843ae6cb0a03f914275db9d1e709e5c62a6812ae12b44e9ca84083cb7e7f8d121dda04a000b134569fc7248b4a80356418cf7ba2cf213b84fafbd6296a19d5a254f6ed74fbdda18e0c8bd2d8499aee19e742bac2ebf1e9b72df48ce459c58a0df7f9db9e178e7de1435cb3e393cb8a1782e561959505d7f00a9112368f1f6075729670d9659ec12e07d9e210a3e75eaf9aaaae7946dc3068a9d4ace3dd5ab287a7fd6ba5b3bdd76fe3f7919f2d656f2d945bcf1dca9b90bcda4c3d8aaf88da7cfead0942b2a046ef13e775c2b066fb314bbd8aaa7997d4f6272333381e1e5a7c2285b00ccaa67c8b93ad08d03a90538926c171628af3217a5db1c7070742c0aa7bfb599453c0649de848fbe654eacd90a3d2d340fcf0054f9d20edcb232d641077bd2f68631fc9490febd6c6b354469bf756b2101ceec376fd02c34c495734a2e91951087334e3220dd3f52933294a2c5b7c1c8746ca32c66a3ccbab17ac5bc0beb448fb8ccdc02ab64e1c6bd85779f3855abd296b119add1b397c7ec1d70bb7c5d2834263d079616d5dd2eed8076f222e4f702a8cb839ac08948a83ad5f5ad998854776780a4bb09e146da6b32c041cc8b2cb5d974cd61eb2660bb89dc8044c4bf7c6f84a28abcb8f97a431a65d520d2276d20c80c656bf3137e4dd731572447565b9163aa131d77370c5a83d9454b756b33275a04acf831e1adb075609285bf89847edeb276f727e4660e4493a6f6d9570f3c24199b32030cc96b334c741b51b09448cc646b9a599eb9e8d55333f928aced84f0ba2a23672ac1803d0bf3f99a6685a07bc85d6eeb2ca7f514d660fa153909a27258143b8e459b582af260a0548ac76313ecedce637b237ca692bc59154d1bd2891be8a9918c9d3291f52935d864103a2ccc30fe2a40a48406c5624d2060df400b9f95d032cee9de12e7ce9858eda9ec8772e3f1670555e5a06a046de3d10d80a86c08d8ee1573b3d0b587005cd25195d40fe98150a932ccc97ad85ea1a253b6c15337b1c5edce251cc3ceff3015b221c4a04de5aa4545c9ad14bc692a8a363b4308f76286dc1f2f0dbe14bd6e8205c1a9560f3deb31bdf49397a5a49f7d564218924439b78e2499db1d6e8f250b30fe4e3aab56ab49777628c2b5b5a8e0e454969f00f3627763f12f0e7c20035bb2ab67fd7b2846e2c8d5a51e0a229dd1964cb5ab2b92fae77f1386860060503baba87d989a76d9b12871e9bb4e4aba90c7ff2c1a7a30aeebfcab567f07809901b0c6c98919c0fa85da8708e0b0fae2c5e24dd685e935ca5e3c9a76f698ef3c4abeffba341d4827881c37085e500318638875331f30d18ab4c71d2be327cc4303b381c4be68db1f1ef5da60e8528ff45ad941c3912556e82b2247ef6bca61b81e181780b2e2730f6b1150a9302ea3bad8a2c98d0e6c3d13c873fbe71f50f1ce02e9350fa3a8d7a03ffb379968aa40e12c3eada19dcabac30042737e2c1e3da4bca0bce4c1ee9aaa1fef38110b159a51a58ef7200dbcab3c464d3b7dd95c22bce1fcb4bdd49ad8cecb257b4aeb98fe1238ddfb2ae5e63eb142672c5c739b6fa4a0041845c6bfe27829c5988e2c43b38c433021c5591d7af8e048b785efd75e35ddc5e003ea1cf7ea470b780a9aef71cf317d7e0926c3af8b2983a6ece8a292018c48541fd8cb1482480464c38e205dc44c2b59aca2c4fc70ba09fc3a6fab4ce41f00ad96a9a641418d311c66bc0433939edb1da5456c00e3b433c8aa6bf04ec6891af79690aeebd0023901d988a0705a5e9dc47a6603f56e122351ab69494c83486927c92ecd59303abf41abaa897a738b9eaf302005cb45d9e8bb9086cd45af8b15e053a6561b83e2e751706935155c8bcead583159cd1e27e9a7c9c071b25588a2e21743787cc1024689a734489dbdba7ee7144023e4d18ebca8a5e79a9885d70415875cea3fef9dd764ce751b58eff3f5c2864e148ba5922de0aab5e32fd4ea3bf3a8cbaeaef9486189cd32babe19fe4a49c625ce235077d5ceff590f9905344a63c768eb5c02506bb0eb7e074999335d059563f38487b7e86f69a47114cb87c5c96e98e54d9982affbd8dde8506e91bd0a79445ccfbbad34a40071d5a53c6947a415e4ea877647551088cc8e1c5921b59589ba9e4fd1a986e8a0a32e0edb471303e9f30d88025ef7f2146e3c280c36ca7b8f237963c9d885b8fbe6f51969a50441670a0d5f231cae445f8070cca78af8adb0b61d569769dfa424848b7fa92fef9cca2a0ebed62f1f3530f486fa6ca54615ce63aa9598baa34accf1fe13eeddddc66259d8f76b2f3a1efde18ce60a41949edad7af2b666499fe4d98a93d8887d29e25123d2148981e985e822e95a403f04ba5ff11243e298653733f2234e4367c9a510b47465338c4dd56b0ce7cc12fc520ed42d07e38ca06f7fee0a8a2c13216b83f813f8edca8299e892cc9da6c1c7f3660c0219a08985f53a7b285c1c3ccb2a7259df74b59d006868ce0a9a7a785642d2e96ce698c383824530cdcb7885beee68db79c60173ff5116cd5a977aa0b52587e5a0b5165db2b74214048866e226439a4f751ceb442bc3c5378c05d1e240ecb21d2d9438585e437694cb51123183b983b5ab59f52609755ad5dfeeb1b3c01302e2906cf83d3895e58e316c6dd5ee8466d4ef11604cd931a1bd8b516f180681dd7e310dab54ea5a91640dd50169b3dabf6e34359e68ca7a559d2a0f4f1db9c6a283809e88c0f5540e43c3c6ec79f90a2e1e788f7360308cf7acfe864dd208826e2ce64a3c65edca137a94c7b8a42abe6b61de2fa5792c5ac8632df159307b90bb4bbdc7dae79347513b2b2d566adb5c1266aee56350fe8b3d75e44b116bb6e0a50d0c1fe0faec47defc88c674b9b4313d0b152bea6af9beb19775bcf6a93e6db8d9c4f48817f2b1d9dddac9ddd99cf4463d6c2cec91326bbf018649ef4017f80d63b898256d00a5b0c9275a9346168ee4c0f170277d24f6b84ddedb553dc71413c7991f2c809fef5a745051eb590399edb84de8afd4f3bcdc9eb6bfc33c42fe42f8fd197241eb72b3f415a0024637634cafe9b87d1cab5532b82b7b0f15e84d9474de5958925d98e7f17adeed53ad3dc7dca3aef2388eaba282442ed141bd0f0e8fc4c865d8a57766e2b2e7b75f87873b1035128b7c3ea2e14ae19c9aec70adc63eaacc547397cd70c6cf4219a7896127466099372cd41b8d9d83f9d1135836cb146958c213266f2c808a0a332aeb6e56083e573b30f123f96508c28e06291bf3189b7fcc709fc82898c895bba48fabeca70920543d64177d38dc662b3f559dc7028071b0a5cdbbd4967999d974ac047f9a4aabd9412de1ee2089f4079f0c3fd4808a15db3c8934d14fff240105cf3145aeb98d86d449bb121d9fca81c3782426fa7da67dfa6b49cdf3a5c69382a2c175a35f108e2fa16ab8e13cbb803bc53d")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
						bytesutil.ToBytes2592(key2): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x60155530FCE8a85ec7055A5F8b2bE214B3DaeFd4"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(35000000),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(40000000),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Config URL File",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "./testdata/good-prepare-beacon-proposer-config.json",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Config JSON file with custom Gas Limit",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-2.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: 40000000,
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  false,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Suggested Fee ",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: nil,
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Happy Path Suggested Fee , validator registration enabled",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: nil,
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "Happy Path Suggested Fee , validator registration enabled and default gas",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89A",
					defaultgas: "50000000",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: nil,
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: 50000000,
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "Suggested Fee does not Override Config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89B",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			wantErr: "",
		},
		{
			name: "Suggested Fee with validator registration does not Override Config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "0x6e35733c5af9B61374A128e6F85f553aF09ff89B",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			wantErr:                      "",
			validatorRegistrationEnabled: true,
		},
		{
			name: "Enable Builder flag overrides empty config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "Enable Builder flag does override completed builder config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config-2.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0x26d85f4c9e2d596cb26a7c6c800a2a500c006369cabd37083d81ed794ef10881b9415b9c7c7dad15bd2dbb71a05996beb9bbe4fdc0c327a41fe9cfe14b7619cb9fc747a59a00816873be712e7e4586a5b0378eebc38bd16681540fa89001b599bc4f6fb26742376978d7593160938ef7b995f1d37cf13fba3901b2cb93bfdae8bb85c63aebdb43871d9fa30d712e7503c4a62f031f2837f1b40740eb31134aa5dabcc3e1141554b12260083675fa1c4d1006970c0763726a74aa579a5381c9268b109f891f982587266b08a1f825485fbe992f9287c0f9e108b65f2b13ee58c4a2bce70ff3765066480b989137769de418a1977f563857429a172514faa7c1da0adc492bb884644d0dabe0f64815acc8cdb405a19dbf7df5582c97de1cc25195a76d342165b0ae26508844dca2ca527a3489e035a15ffed53b1e24b4feaf0c97860cfae172105e12958e9ee58ea76e6b84cdda7f889fbf53cd728f66325719f917e6d060c9b81f32b92b061214132b83544c8d20bdbb458e0ca25146ef0fdcdbc7b25879477deffd823aac234f23bcc0a5b8b73cd752906f26c4493cda9224fb1bd36ddf8df5b79113a03d00bd071d8e708091767f2924c959f4a16b5cf55c5eaf447d8902db2fbf27c09a74324f121a3f5393362bcc8732cbcbca46f421f3666ce0bca0064628444f4499be7c88b9624c34e288548c1bee4c32c25f5c1a44da774f77fa765023e594e2ea4ce46dd8d1ba5218baaf8a31e0793e3e76684726eeb89d73c24edb766101b259b5138f8124eb65e8efe3a620ee90b547a3c908bd12265ab8f68dd6171d54e2b86d03f78d648bc778eefc681da5271516650e8890acb0a3516a96a03688cd1a6e3e4c4ffeae23d1735059036ac6b1e95398131387b9586bcebb8452d8ebe2582c7d8a7b48a4adaae215cb0d91548545c6ae3f970994d3473afa711d5f8976d4ebb44b887bd5f29fa7f83db506957e66d6d6674d4b692bd1e337be5f0e55d36f85580377be0ca8fc59e21f5bdedf1cc51dd3db608faaa52e6f9313f18e9e5b24254a4c91bc82326d2f287857f84b26b1d3f3f82919e37ce31679d449eb6edcb86055506c90663b7c8746c2d6173ec45a83d5463e1796fb6bfcd7fcc75dd9298bf9f910725ff23c3bd7a9fb8c3a94153b066a3a46bc8c246a82e2bd35f30926b11d0d0c0965505a1c49a9820cf4658463c6f46714e1c3b5bc778a4d94e1cb90482c8632edd9049f0b141cd8fa7ea9e859e4e334fc3e7d9940e74bafb1beaf8c552519e399301755b5c6864c4870952cd1aed0e6c7eadf6749b033824348a24c32f569a9476df2c06b013e051b7bcf5ce82bcafb191b7c0ebda9caed1d96dca48f42972feb800868ae8af2048322483c3becb1a1cd17fea6aeb4ec1cf7a3a9187aa93789f63a8ece10fc4666a82cdb231728ca89b9b3472e98b508b09aa4d5ee0650d31963be5483e3a676da1b8acc0837af94d6a44bc9c379434da61be00cf817e8fea8273ccf38e955ebfa87d36749cdc209a20f8d6eb75ca677149c92b9ab6965fc654cf89c2b3a0ed46b094da07859e93d6c017a16f209e2c68bbf6e0572a29e049785ef3fb997327cc991f450c175486b7622add67453c26d0dc3411592fc1c6fc384a69039a7c9b29dbb092c81ac8fe55f896edd979b8f4d6add390a6d1ea5e3c69ad1213f69348e8499e596836a471a4297bd16ca432050b14f5221abcd733f7f6d8f9d255e118bb578baf35923bffc50184c94009300d7c065f3e6c2ca74cb0a9046a82d472cc012a9aa874e984614c60b9b75b892b5ca73e3b9ec3fd08839d7e6c9b197d35be688dead1aec92376a8c4ccb2c6f0bdb304706a0d7b349fe1c64f3a4612583966955582af1b6d2b92494fb7f2838e49459cf0481259bc52f434dc811f780c9700de55ead92c51a3765188c78f46a95e050e016f05c6bf12e54e4019c9f44df89e2793f373e43633b4bc9923a5218eca650e767ea8a66734c1fddd9e193c163cb13b0607a31551d075a8fdcab15895248accbb37ca8ed58e2755c92fec29b866be605976a506a3d27a494220e937bbad58a16c4805b7ae4335c55a7c10149a1ab423b16286df04562e0bf6b95067cf9c58a42b95d48c903a608a995feeb5f3bbcbc6b250c32ed9df54a852d14915b7765be9842bc21c225ccb74fa7049b3ecb7c79ff06d6ac815840b03128cf9bb1901ce226c29b366a5b1c79ac7ea4bf7e7496b5dc43d876c64a76bfba850a39a6190de4ab32d1c9866b292f5cc011996ce92376b2d820e8a515dac17db8d07cc58085ff5797436a3099623a94e0e7266f3231788a7bd37e50d76a36c251aea23573339537abc40f255bc62c73997b4b816f52df49709dcd7cb3ffb0563b0387ac0719d3cbd5ec2e0d9dabfaa23784c9f085d5f438e88110f0b8841042fea30723f8f29cbd1979aafa44e4870fc425b3b585b1f821aa33f36ec890ccb58f901d775cfb2bcb264abdabdfa48a9ef1af069ee1c885234bc9610ed0583b2f7a1290837155ac427b187261d56873a74eea95bcce8ade9002433bf1a3b94e7b1dada5c95c4c50b37f22a65a039c2f28d2242a80f57ca480fb0746e5f5ee17e3179f2cc65f887c8c412ff6cf9d2c440713200b6563bc553c1e9a5ab42938b09db8f5b1645f44913c45387c71dee3ff5d48ad2afbad2a366d3bf86f85eead6d306c00de3f1bd4859b4c503267cbe3082e6b245f724d165855eefa8b09d9ea49eb4180cf69994c34f78ce2497a67fa4642a149ad8fcef872c65e6a850d213060d18ef9e476df35e3a2f5086b3c6d72ebf7075b60c9e016ec3f64bf6270d86917a298f5f0b0c5d596b49ead0d52b99b007fac79bffe3414a3b7948236e4c1dde6547d789521146ab4dedd6c20487b5dbd9a61b43de2772e2762b71e18ce1320adc4c1261a7c0deddecc51bbeef6151eb3d9cc8420ed01b59a444d99b3d18fe6342ac966e339447ec61746f37db9f88ec445f9d17ff6997587eb3a4bf913d0f7b50dec0197493fff6d7b9b16d656d40b7c9e3cd4bcc6bc823d281b620dd74e3409a48a3e273609ef966bb796bfec8a372c4dba3c4cf371c00384069c63b12a8dad9d05f68864fb6bbca4d530bcd1b972a5b2553c5fe06ad1f9ac3f763c5544f045e9740bcbc4f2a30fb4e51dca4600501ecf4cb4781401d5b8934e447d707e8110f3fe6b9b9d401b665b11c71b42fcc0dd29946f481dd6535ea29b99512068de634c2fcd9c1247e1909d44ceabf8b63bdfd48a2e85390e25b877acbf2ac15d408189b11b67260c3d6637087409e45e491cf2d12a909099d8037311afc7e54666966406cde8db2b419ed8900d661b3cf618def9c4f6b3dbb1371e665a3377539202c82c7b15d0a90c4a0f7ae9ea92b140fb9905d4882cc33308c255de82aa317f3146c60bed54d5af7093a973eefddc0d7b29f2736df6b94804e8d8bed3c56535391ea930ed34bde359c292ba774ef28f123d404f3df7434c6b1815afb3867eca8b5160c7c7b716e8f23853efe0346dcabd41f2923ffc8e8e5c517b76f0a431818f3b1ace953ace74eaad19b169698ceb119dede7d7a137273cbdb28b1e4277f9f7f5f27847ec2ce70940088bcbc5e697d5322e490f15361d41f3f6eb6875557a4d492daad9")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "Only Enable Builder flag",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "No Flags but saved to DB with builder and override removed builder data",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(40000000),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
				return db.SaveProposerSettings(context.Background(), settings)
			},
		},
		{
			name: "Enable builder flag but saved to DB without builder data now includes builder data",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
							BuilderConfig: &validatorserviceconfig.BuilderConfig{
								Enabled:  true,
								GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
						BuilderConfig: &validatorserviceconfig.BuilderConfig{
							Enabled:  true,
							GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
				return db.SaveProposerSettings(context.Background(), settings)
			},
			validatorRegistrationEnabled: true,
		},
		{
			name: "No flags, but saved to database",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				return &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
			},
			withdb: func(db iface.ValidatorDB) error {
				key1, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
				require.NoError(t, err)
				settings := &validatorserviceconfig.ProposerSettings{
					ProposeConfig: map[[dilithium.CryptoPublicKeyBytes]byte]*validatorserviceconfig.ProposerOption{
						bytesutil.ToBytes2592(key1): {
							FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress("0x50155530FCE8a85ec7055A5F8b2bE214B3DaeFd3"),
							},
						},
					},
					DefaultConfig: &validatorserviceconfig.ProposerOption{
						FeeRecipientConfig: &validatorserviceconfig.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x6e35733c5af9B61374A128e6F85f553aF09ff89A"),
						},
					},
				}
				return db.SaveProposerSettings(context.Background(), settings)
			},
		},
		{
			name: "No flags set means empty config",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return nil
			},
			wantErr: "",
		},
		{
			name: "Bad File Path",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/bad-prepare-beacon-proposer-config.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return nil
			},
			wantErr: "failed to unmarshal json file",
		},
		{
			name: "Both URL and Dir flags used resulting in error",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/good-prepare-beacon-proposer-config.json",
					url:        "./testdata/good-prepare-beacon-proposer-config.json",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return &validatorserviceconfig.ProposerSettings{}
			},
			wantErr: "cannot specify both",
		},
		{
			name: "Bad Gas value in JSON",
			args: args{
				proposerSettingsFlagValues: &proposerSettingsFlag{
					dir:        "./testdata/bad-gas-value-proposer-settings.json",
					url:        "",
					defaultfee: "",
				},
			},
			want: func() *validatorserviceconfig.ProposerSettings {
				return nil
			},
			wantErr: "failed to unmarshal json file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.App{}
			set := flag.NewFlagSet("test", 0)
			if tt.args.proposerSettingsFlagValues.dir != "" {
				set.String(flags.ProposerSettingsFlag.Name, tt.args.proposerSettingsFlagValues.dir, "")
				require.NoError(t, set.Set(flags.ProposerSettingsFlag.Name, tt.args.proposerSettingsFlagValues.dir))
			}
			if tt.args.proposerSettingsFlagValues.url != "" {
				content, err := os.ReadFile(tt.args.proposerSettingsFlagValues.url)
				require.NoError(t, err)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
					w.Header().Set("Content-Type", "application/json")
					_, err := fmt.Fprintf(w, "%s", content)
					require.NoError(t, err)
				}))
				defer srv.Close()

				set.String(flags.ProposerSettingsURLFlag.Name, tt.args.proposerSettingsFlagValues.url, "")
				require.NoError(t, set.Set(flags.ProposerSettingsURLFlag.Name, srv.URL))
			}
			if tt.args.proposerSettingsFlagValues.defaultfee != "" {
				set.String(flags.SuggestedFeeRecipientFlag.Name, tt.args.proposerSettingsFlagValues.defaultfee, "")
				require.NoError(t, set.Set(flags.SuggestedFeeRecipientFlag.Name, tt.args.proposerSettingsFlagValues.defaultfee))
			}
			if tt.args.proposerSettingsFlagValues.defaultgas != "" {
				set.String(flags.BuilderGasLimitFlag.Name, tt.args.proposerSettingsFlagValues.defaultgas, "")
				require.NoError(t, set.Set(flags.BuilderGasLimitFlag.Name, tt.args.proposerSettingsFlagValues.defaultgas))
			}
			if tt.validatorRegistrationEnabled {
				set.Bool(flags.EnableBuilderFlag.Name, true, "")
			}
			cliCtx := cli.NewContext(&app, set, nil)
			validatorDB := dbTest.SetupDB(t, [][dilithium.CryptoPublicKeyBytes]byte{})
			if tt.withdb != nil {
				err := tt.withdb(validatorDB)
				require.NoError(t, err)
			}
			got, err := proposerSettings(cliCtx, validatorDB)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			} else {
				require.NoError(t, err)
			}
			if tt.wantLog != "" {
				assert.LogsContain(t, hook,
					tt.wantLog,
				)
			}
			w := tt.want()
			require.DeepEqual(t, w, got)

		})
	}
}

func Test_ProposerSettingsWithOnlyBuilder_DoesNotSaveInDB(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(flags.EnableBuilderFlag.Name, true, "")
	cliCtx := cli.NewContext(&app, set, nil)
	validatorDB := dbTest.SetupDB(t, [][dilithium.CryptoPublicKeyBytes]byte{})
	got, err := proposerSettings(cliCtx, validatorDB)
	require.NoError(t, err)
	_, err = validatorDB.ProposerSettings(cliCtx.Context)
	require.ErrorContains(t, "no proposer settings found in bucket", err)
	want := &validatorserviceconfig.ProposerSettings{
		DefaultConfig: &validatorserviceconfig.ProposerOption{
			BuilderConfig: &validatorserviceconfig.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
				Relays:   nil,
			},
		},
	}
	require.DeepEqual(t, want, got)
}
