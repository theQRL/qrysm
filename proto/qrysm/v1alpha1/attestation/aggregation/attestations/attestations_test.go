package attestations

import (
	"encoding/hex"
	"io"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/encoding/ssz/equality"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation"
	aggtesting "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation/testing"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
	m.Run()
}

func TestAggregateAttestations_AggregatePair(t *testing.T) {
	sig0, _ := hex.DecodeString("a4096de23667f9a74693456e0d7c3923b0a8a859924cc17b694b25a5b09130552213d0930d19cf93523e922b68e05e5db64501467aa81a69f4bec186534151807a106549ad577dc164371ed8b1a8d64cbed0d12a2f3669dc46106ba54ceaae73036ad67f8e02773edb33fd9e0ea5599fd8211d7deb995ff6320745f9e7f460161de7e7a7f25b6db17d541837c23ca1b8a4a2c215f61fbe4050d03a2c88d8a8fde2a69998bb8622926b46fa5b76299babfdc12b2877b7b18d30d516574552a6a83f0d1b9fe0888b0f8d5d121f05f7f745732df8ef80e3cecfcd4f45420a64a393b0f5d0f5beec5c95057fb6df474c947882e0980ea3f0ed494f654173809b5eebe7e8ec34bd9885c52c319b85b5c645254690c4ab807bff7f21ea01cd79998a6374221e8a2f9c9ec8a05876996f72820aaefeba6b9987bcd8e886df35fd1543d76bb2291593e8de7ceed48e46fcaef9c76396b7facdfeb8c97d19b85d381ebbfd419f7f9a93b4a03a3771aab1d1d07742d6f0e9c0b8ff80e437dc7953b381aa7501bc18f42a4f7576fda725c689a7421325cef139ab3fe3fb99923bddde8b58f8d32ee114df5ef4bfba571894d83d0f9070c74a20e6d6dd32eeba75a0f31260823f45cecd28f9560f7d72b0725d0d54b4d9aab2710dea1a09420f4fa344c50798a003d14ed1c2fe76154d1312e686af0bcc6c02f6352e0433beda843e02c47270a7fe2997fbef0dae955527ccf01d3df43f052b6bf5aea18452f685d3546d1016dcaac71b1914ccbddd475557d0c60b0e0de85822ae1a0a09321123af1ad6992dffe0683ff4beb3005d77e05dedcd33b7b6a700566d13c3e4c8d30f21653c4b31027b2e4a75f6b815d9fe7b5e2e55a761101564d3ce402638898d8b0e6c913adcca7cd60b48bf62ddf845569e76f668e00c804435260285fafd85ec6bf365b65819256197fae8b60385f28170d383e6f837bc7412870da524e281c58a8e21e9ad22faf0877613bc4e2cd4e6cd6eb09cdabd9db4af0dca147d72c43d56568ba879332e8c8ecd7b067e99350f2819e9d4bf9018db0e8deaec267ddf80f1c6ebf51244f7e7b334e90d1af11ba02469658521936f0368c66b6184372401b6a3b43214a9a0e08c4f618f4c96824270e60bc2a201a5efc8f5bda3d88a769a839564fab75d7f12f67d52a32052a31dbab60096f1ec5b67ade1bf1acb4db93dfc79bb62d89ee5991f0a1a2bdab57863007575f672b53d802b0e021d008c7a24dfaaf198abfbbf2e02200157127d0545b57ddbcb44bbecc96069bd4f1c77b232cecdce27bdb483c5f1460fda8a0fc2972d4d85443d8d8b96926cd98477257840aff096b7455e3db818811a8acb30fdb651aa4471656d48dbdef533242abd6d4782fb9b99923ed0d249023166445ca1dddc5bdc9179b3e567a37bd53a7cc93db116b3f869a7066284607caa5f13c91bf650bdde73581d9610ce35ca01be91d02ae38e3ff9b59fd0380e9dbb22a4ed75779a69ba6002021cb6cad385b4fbbbbac1e7139c209fe82c3213ebb67dd035b9bc28e1d970bc74e58389268137b717560ed77a63cac40560a91a138518421b4511319ff05c49639dfed6962480955aec5283366ea13512231babfcf5bc0572e3cc664d53843b8980e11ce6dd43edcaac47f53b5a09df41fa7d45f67e4344c912c0943735116e5697461498321c38274e3cdf48108bfb9ffb7595f2d9797f62ef1ec6183c326740874b5c10276dfe845ce734e4166cbe25d610c7ff106d0ea58df95707b797b6f324ff06e994521f8c1fa8f0c3a196c177d8bb3afff22e3d355beedbe682d4dbbfdd3baf6170634609da868796fa7becc06d2a1c3105fa6140380503e444269655e0d4f7c247007dd9ccb74ee82db257ff044bf758aa053a871433932aef38fb0ed34e1940b2b7ca57e3c2811bbc7f682d7e314b67d03faaea50737ca26d3098074177c818afc14435e515309a08f00c9350d0133f1cf5aded4c486c6f189244353281212f28ba669f2e0719e194604674ab67f5dabc62a61ffa8f4eb08d12c1d7b0e31656d406901347a2832fa2dc941c63b5940fcc31195f4bbc7e8a32fc765c6113fe157bef846b40312bab68c7e94d9248411b667891679451e2cbdb584300846798bedd13ecccf09964f10a0b8a956afd7b1646d6ee34ba9e0cf368756a13460d38f83a429e947b14a54030e530a2907f1f266292e92e0dc1520d8901f97e861241438bd4d387980d37fec60b9ab5cf150527e1497b77abf7c334dce524fd1b8a22e36ca2527639b9886534948143f7894388685de0d205a5b93026e79c5d1e430a12fe1c67eb95259774b8698287b53e512f8acce30a8030387adb9edc14a4e1308c8c75f2086a4f94643a8d737734fbf7214a763c2e549372fd5e2fd4f8fa587a58d870d169fa97f40b83286cb8eeefa12c593d4b1d9f2d3269f8eee3cd2960d2fea37bfe7bf2a2ccd3f0a3be0e8147f9decb9c1a045125d219e0509271e3e3dc8cb2aad2227bb8884f38f042feaab680e67763f4830c398b9a711f4d4ce43c11e0ede82c3567106ac9989fb9111beae74e0f0059c12a308ad66ad677331789e272e84a917062b4335d4abd911d77c1537113477cc2d12fe47c6733a4a5ae254f0705b74d1531c1ef121adc6035c60e49b3bc8275b7de930b957659085ccc290f71888497a6a61fc26566da3a9b66d7997f2b605c2facf321b463f10a33cc913d20a40e86da7ac57899c69458f4a76a6d5c1aa6e99017a08ff37e2652aab5286ecf465b60fb40cde3f921f73015984893c47a6bea4bc7efa9ebd4b9ea310dc2790a5ff7d5dcaa9b5ec730cf9551ed70678d0f4c8d2acac8d1f2ea7304dc90691d0dd98099150d8a318184a4b76dfd0b7cae2569178061bcfbb69a30200901a4fedbcf175490dfcfd267caee2e90d7406c45dfe5419cb5dc4add7bf32361cf3fd1c5770ba09ac290381ae2bb9f520772ab089d7e985af74c16f73ea84f4475eac711e097cafd9596f9bd91d48c31a8c1b13978b795a845aad85f3fe08fbb825f4b81ec0b9d12483a2676050b6c6a51290241a8ee3f13adc57fb36a0e920319dc7eeb252ac71e5c6a771f3990dd83acf38c24be0c6c9df1ffc17084cf0f10670945595787cfc3d19db0a6a14820bda56021b1400959b57ccf0d096ed43b75d7ed0194096343e831040dfe8399393e9eb7704637c33cfb7fe6b62d59d99e59e23c6973944d3e5a2a9c2bef3c6f456783ab283cc75b01b241d3033f9b59441857c626a4be2afad85450a1267c426d39678c81da11bc2968bd6b025d7e128964001b92fc7d2563b7f0d024a6d07b6b7e88cbe86f2ddfd7337346e52947eb582c780d16b397fe06bb980fba933cc7d84baeae379547aa1af28f2576c26c6bd7f1893e4da32312848a038fe4df70e55efa08409bb369d2d80882735f1e8d359c4545af6be3abdf4755570b395aa818b82cff0cfae5b046d439c0e35a9841ea95c9abb1db9caf47c49a1bdd83e954db19af9dd480b8721307edfe28dcf4a4c9824a4cd84b840e19c265735d327d0ec138dd4a4ad5e9ae3c48412210455aab6462524b6e64a191c9c14e9ae6eb4d046bd4c42533bce9d22faa1b2b29c3dafa450d29bf261772bb287e1dd364e1b890593758ba88973a7fa2a6b221a19e277b641a17a36b5b5ca1da9fd45339162e526894d3398cfcf41aac9787000085664012485ba31325d826ea9513036db221730035b8b11771ec990d01dcd58e2803fc609428cd66112a6d4cb1d690586405a1b6a8ba4da16fb01b9407446714928bd9b05856fd6d8a9a403f4b85ee223f8680060fc6b0a8a62c37b69b3d69bc6429d73d930f7f72fee7ebf096b8253cf7bef53f933fc1f1f44197762af3d34d4ee7ec99229f3869a2017c6d5286c6d933c255edf9e9404076751b328d685ae6843ae974589b0e87d66371ca8b31366b0d4890e879b012404eeb6c90f3ab70cbe8999bbdc8c21313b71ea7668130fce7e043140d406926a437e564421c0c58f3562fb8b31d395072dd1c71ee34df7d4626babdbf0e5cdc847ad3e4323e5f8c7b842ebeaa6380c3255558785613414d9d07ed37f1e056213ffe318a4a46423d849c6150f53f3acd5bb7854294b5e08183f89a99be5d9720c44de258d55ef8d67a77b640db3b90ceb27722159839fc3499bf00d6dc69f10396db75ec688df871989f205c0377d669fa11a2bc3f3cb95ab1f03c365e04184ac388eba2684df05e667b7964ecf5370bfa3ad2b15ac2ddfc78a51e8c6ac2a4f77a55bbc99d1984e3df38cd357efd8343a805526a03711bf1bb775e9718c32717e00e7d11aab6a4633d72bd812f30395eb0bbed9094126366b8d95a955989a0c239778c2fb5935e2328a29cf04decafa1bc93b2e4e67864650aa55f7038481e12c10344a8209278a70ab844ac01ea51de2c448660dc99ce4778f6009ca9f44e6acda433a455f88fa60af13cd26a7fc66a17c2bbdbde87b4dad0e44b684d7171cc50063098385c46e92417eac40d2422a567d39f8a4588250ac471accc5b97510ca741966fb258e02600cb2a1e783aa6482680a3d0b43fdd1999121a15340189f00e69066d99fffd267ea0edca3f9ae53f0f05e9c9d6c60f3a717faaf8f1c6699a491fab0e60df8db8a955b93e03c2f2c085ca83d5493e9a2f20006280316ecfa57b0b8006a4f64732647ed6d68052a30edfb3915df1de7d55155db0b5c0a6fe71e10a4fa222cd45204b54847795bb0da968728764884fa40346098f7e03469a99a3313784c928b48bc3ba81aff577fa00e9fd8c532bba6afffad021c69810997742762fb3c82529a93a51317933c6e641d9f7537fddf5227385ee858340f8d43d28052b6ef3138acbbcc4469026788101cec334be3d918064888242aeb4f7fb615f058e4f6d3f89f0481f98e132e85bc8bcc93f5535a72952b2ea175e33eefe532cfffd2b61f3a1a6be9df3810c7d869e25531aeb75b9a9fb09461defd567328acce0800f3b4c21d3fbb1f4cbb099da497089de8f660d39eca113cb1989c3b9969c64e8fb4f276ee0ca28453232df4039c8ea612982612c0da868ec3ee511a0552dd74175959628e6279524a8a926c9ee86e23398fe9cc3d9df94d4a84af3e106c42162955cb19443ca0c93bbf1bcc798d38ebe8180cba74e6cb4baffd6b74e0bf4729f1e55bb837d499cfd11f4eb6718a9a1b71fb5cabb8fb6e26b7dc94c3e8e16b2ca476ee4109eb42c4948cf97d76a0aedf2facd301584825142daa60c4923e65b9ee2b4ad5f326544bf0be470093059b75d7059944e4ef04fd2cb3d22655087f8ea3776832ffa92609bab47765d71990a82fb873f257bb221e2e00692949c8cadedac7520318d3574934ad94f80ffb2debff0dc4c93eed28f6c59498cf3e7b930bff776d55b22f4968ff8b333b32498d77931059f411e65f692095118eb75692f4ab11e9390e7b456e936dad287b7f13a450f385a9c5c150638762253f1288b76fd9f0f0a17ecd6ba09875fc9f8a6998369b0baf2893ddcbb173587800364749677e40bd0fb14eefaad79761ae715069fc303f428155643a07440c62b81b29cb93478b402ea39a28beec860caf7a70c79932de9caa78b8397704e825e69ce6125d2e8f629f0f91e090fa34a8218c25b0c3211f4acbb2779665efc1756f6ae35aa8d1b7ec9aff01c10983349eee319ec901c25fdc6a0c3807ce4257d52464e7fdb05a385f0613c989f323df0370360c682012a7925d4c8b1550380a9ee2ce666d6d4a8cb2a4abd2ede4e7cbe879d52dfc1e455baa4d9a3c9d850faf30ac327bb2d167c0fc088f52f521a8bd71c3810ea4919f43e4ebdb8a075b0c7e17da7eb4c107e6adc2a657a0d21dd083e7f5d33c2b84221a4261dcfe2ce2b6056e2b8024393ab4fd9ff195bb6a80ebfe39cecde287b0b9324bf45199dd410ec936229ce2b1c5a802c2fecc38e87b1c50b579dc14a484ea7c61d80142572b389684d8d6eb7665a14c3b716b8e9b3f386c9347c17eec588603fe330816a34fe80ec9012f5667a2b3f89a0194105ccc39b9656cf95ac4b68a71845a0d0cbeec596dc54cdea4601786fb21fca87882c1b9bb813a3f3d153898dfd042b49dba16aec040acc5eb9c03e401914ddd5077f65f736133da3061ef95581c273c9d68aec2e15a98059b9caab40dcec0172c3dfbecc3c8653102595ec1abfdedb53d4108488d1dff8dab7eb78398d8eb4069bf75dc76b8d340f7c83fa9fd5059bd1929d2bcb74a83a3f1ff0572a116acf8c5e5074061acc652232ab89b90125b3b14b621ebf1a070375fff584b51bc86b6d1ebeaf93a6d44d3c7b60b6509a680054668425cbf1c10e227e95cae4f5184a56a9b8d4e705247987ef144298af070b164eb7d2060c0d212961a9bde4ff10142e303a53568083afb2e84c5f6f7a95c1ced300000000000000000000000000000000070e13171d27333b")

	tests := []struct {
		a1   *zondpb.Attestation
		a2   *zondpb.Attestation
		want *zondpb.Attestation
		err  string
	}{

		{
			a1:   &zondpb.Attestation{AggregationBits: []byte{}},
			a2:   &zondpb.Attestation{AggregationBits: []byte{}},
			want: &zondpb.Attestation{AggregationBits: []byte{}},
		},
		{
			a1: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x03},
				Signatures:      [][]byte{sig0},
			},
			a2: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x02},
				Signatures:      [][]byte{},
			},
			want: &zondpb.Attestation{
				AggregationBits: []byte{0x03},
				Signatures:      [][]byte{sig0},
			},
		},
		{
			a1: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x02},
				Signatures:      [][]byte{},
			},
			a2: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x03},
				Signatures:      [][]byte{sig0},
			},
			want: &zondpb.Attestation{
				AggregationBits: []byte{0x03},
				Signatures:      [][]byte{sig0},
			},
		},
		{
			a1: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x1F},
				Signatures:      [][]byte{sig0, sig0, sig0, sig0},
			},
			a2: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x11},
				Signatures:      [][]byte{sig0},
			},
			err: aggregation.ErrBitsOverlap.Error(),
		},
		{
			a1: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0xFF, 0x85},
				Signatures:      [][]byte{sig0, sig0, sig0, sig0, sig0, sig0, sig0, sig0, sig0, sig0},
			},
			a2: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x13, 0x8F},
				Signatures:      [][]byte{sig0, sig0, sig0, sig0, sig0, sig0, sig0},
			},
			err: aggregation.ErrBitsOverlap.Error(),
		},
		{
			a1: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x0F},
				Signatures:      [][]byte{sig0, sig0, sig0},
			},
			a2: &zondpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x11},
				Signatures:      [][]byte{sig0},
			},
			err: bitfield.ErrBitlistDifferentLength.Error(),
		},
	}
	for _, tt := range tests {
		got, err := AggregatePair(tt.a1, tt.a2)
		if tt.err == "" {
			require.NoError(t, err)
			require.Equal(t, true, equality.DeepEqual(got, tt.want))
		} else {
			require.ErrorContains(t, tt.err, err)
		}
	}
}

func TestAggregate(t *testing.T) {
	// Each test defines the aggregation bitfield inputs and the wanted output result.
	bitlistLen := params.BeaconConfig().MaxValidatorsPerCommittee
	tests := []struct {
		name   string
		inputs []bitfield.Bitlist
		want   []bitfield.Bitlist
		err    error
	}{
		{
			name:   "empty list",
			inputs: []bitfield.Bitlist{},
			want:   []bitfield.Bitlist{},
		},
		{
			name: "single attestation",
			inputs: []bitfield.Bitlist{
				{0b00000010, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000010, 0b1},
			},
		},
		{
			name: "two attestations with no overlap",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000010, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name:   "256 attestations with single bit set",
			inputs: aggtesting.BitlistsWithSingleBitSet(256, bitlistLen),
			want: []bitfield.Bitlist{
				aggtesting.BitlistWithAllBitsSet(256),
			},
		},
		{
			name:   "1024 attestations with single bit set",
			inputs: aggtesting.BitlistsWithSingleBitSet(1024, bitlistLen),
			want: []bitfield.Bitlist{
				aggtesting.BitlistWithAllBitsSet(1024),
			},
		},
		{
			name: "two attestations with overlap",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
		},
		{
			name: "some attestations overlap",
			inputs: []bitfield.Bitlist{
				{0b00001001, 0b1},
				{0b00010110, 0b1},
				{0b00001010, 0b1},
				{0b00110001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00111011, 0b1},
				{0b00011111, 0b1},
			},
		},
		{
			name: "some attestations produce duplicates which are removed",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
				{0b00001010, 0b1},
				{0b00001001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00001111, 0b1}, // both 0&1 and 2&3 produce this bitlist
			},
		},
		{
			name: "two attestations where one is fully contained within the other",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000011, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "two attestations where one is fully contained within the other reversed",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b1},
				{0b00000001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "attestations with different bitlist lengths",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
			err: bitfield.ErrBitlistDifferentLength,
		},
	}
	for _, tt := range tests {
		runner := func() {
			got, err := Aggregate(aggtesting.MakeAttestationsFromBitlists(tt.inputs))
			if tt.err != nil {
				require.ErrorContains(t, tt.err.Error(), err)
				return
			}
			require.NoError(t, err)
			sort.Slice(got, func(i, j int) bool {
				return got[i].AggregationBits.Bytes()[0] < got[j].AggregationBits.Bytes()[0]
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].Bytes()[0] < tt.want[j].Bytes()[0]
			})
			assert.Equal(t, len(tt.want), len(got))
			for i, w := range tt.want {
				assert.DeepEqual(t, w.Bytes(), got[i].AggregationBits.Bytes())
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			runner()
		})
	}

	t.Run("broken attestation bitset", func(t *testing.T) {
		wantErr := "bitlist cannot be nil or empty: invalid max_cover problem"
		_, err := Aggregate(aggtesting.MakeAttestationsFromBitlists([]bitfield.Bitlist{
			{0b00000011, 0b0},
			{0b00000111, 0b100},
			{0b00000100, 0b1},
		}))
		assert.ErrorContains(t, wantErr, err)
	})

	t.Run("candidate swapping when aggregating", func(t *testing.T) {
		// The first item cannot be aggregated, and should be pushed down the list,
		// by two swaps with aggregated items (aggregation is done in-place, so the very same
		// underlying array is used for storing both aggregated and non-aggregated items).
		got, err := Aggregate(aggtesting.MakeAttestationsFromBitlists([]bitfield.Bitlist{
			{0b10000000, 0b1},
			{0b11000101, 0b1},
			{0b00011000, 0b1},
			{0b01010100, 0b1},
			{0b10001010, 0b1},
		}))
		want := []bitfield.Bitlist{
			{0b11011101, 0b1},
			{0b11011110, 0b1},
			{0b10000000, 0b1},
		}
		assert.NoError(t, err)
		assert.Equal(t, len(want), len(got))
		for i, w := range want {
			assert.DeepEqual(t, w.Bytes(), got[i].AggregationBits.Bytes())
		}
	})
}
