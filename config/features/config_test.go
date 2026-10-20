package features

import (
	"flag"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/urfave/cli/v2"
)

func TestInitFeatureConfig(t *testing.T) {
	defer Init(&Flags{})
	cfg := &Flags{
		EnableSlasher: true,
	}
	Init(cfg)
	c := Get()
	assert.Equal(t, true, c.EnableSlasher)
}

func TestInitWithReset(t *testing.T) {
	defer Init(&Flags{})
	Init(&Flags{
		EnableSlasher: true,
	})
	assert.Equal(t, true, Get().EnableSlasher)

	// Overwrite previously set value (value that didn't come by default).
	resetCfg := InitWithReset(&Flags{
		EnableSlasher: false,
	})
	assert.Equal(t, false, Get().EnableSlasher)

	// Reset must get to previously set configuration (not to default config values).
	resetCfg()
	assert.Equal(t, true, Get().EnableSlasher)
}

func TestConfigureBeaconConfig(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(enableSlasherFlag.Name, true, "test")
	context := cli.NewContext(&app, set, nil)
	require.NoError(t, ConfigureBeaconChain(context))
	c := Get()
	assert.Equal(t, true, c.EnableSlasher)
}

func TestConfigureBeaconChain_VerboseSignatureVerification(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	t.Cleanup(InitWithReset(&Flags{}))
	for _, tc := range []struct {
		name    string
		args    []string
		enabled bool
	}{
		{name: "default"},
		{name: "opt_in", args: []string{"--enable-verbose-sig-verification"}, enabled: true},
		{name: "explicit_false", args: []string{"--enable-verbose-sig-verification=false"}},
		{name: "legacy_disable", args: []string{"--disable-verbose-sig-verification"}},
		{name: "legacy_false", args: []string{"--disable-verbose-sig-verification=false"}},
		{name: "disable_overrides_enable", args: []string{"--enable-verbose-sig-verification", "--disable-verbose-sig-verification"}},
		{name: "false_does_not_override_enable", args: []string{"--enable-verbose-sig-verification", "--disable-verbose-sig-verification=false"}, enabled: true},
		{name: "dev_mode_default", args: []string{"--" + devModeFlag.Name}},
		{name: "dev_mode_opt_in", args: []string{"--" + devModeFlag.Name, "--enable-verbose-sig-verification"}, enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			for _, f := range BeaconChainFlags {
				require.NoError(t, f.Apply(set))
			}
			require.NoError(t, set.Parse(tc.args))
			ctx := cli.NewContext(&cli.App{}, set, nil)
			require.NoError(t, ConfigureBeaconChain(ctx))
			require.Equal(t, tc.enabled, Get().EnableVerboseSigVerification)
		})
	}
}
