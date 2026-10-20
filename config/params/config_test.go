package params_test

import (
	"sync"
	"testing"

	qrlparams "github.com/theQRL/go-qrl/params"
	"github.com/theQRL/qrysm/config/params"
)

func TestConfig_MainnetAttestationPropagationWindow(t *testing.T) {
	got := params.BeaconNetworkConfig().AttestationPropagationSlotRange
	want := params.MainnetConfig().SlotsPerEpoch
	if got != want {
		t.Fatalf("mainnet attestation propagation window %d must match the inclusion window of %d slots", got, want)
	}
}

// Regression test: DefaultBuilderGasLimit is what validators advertise in
// builder registrations and what the local proposer targets. go-qrl rejects any
// block whose gas limit exceeds params.MaxGasLimit and the beacon node clamps
// the proposer's target to it, so a default above the cap can never take effect
// and only produces invalid registrations. Every shipped config must stay
// within the execution-layer cap.
func TestConfig_DefaultBuilderGasLimitWithinExecutionCap(t *testing.T) {
	configs := map[string]*params.BeaconChainConfig{
		"mainnet":     params.MainnetConfig(),
		"minimal":     params.MinimalSpecConfig(),
		"interop":     params.InteropConfig(),
		"e2e":         params.E2ETestConfig(),
		"e2e-mainnet": params.E2EMainnetTestConfig(),
	}
	for name, cfg := range configs {
		if cfg.DefaultBuilderGasLimit == 0 {
			t.Errorf("%s: DefaultBuilderGasLimit must be non-zero", name)
		}
		if cfg.DefaultBuilderGasLimit > qrlparams.MaxGasLimit {
			t.Errorf("%s: DefaultBuilderGasLimit %d exceeds go-qrl MaxGasLimit %d",
				name, cfg.DefaultBuilderGasLimit, qrlparams.MaxGasLimit)
		}
	}
}

// Test cases can be executed in an arbitrary order. TestOverrideBeaconConfigTestTeardown checks
// that there's no state mutation leak from the previous test, therefore we need a sentinel flag,
// to make sure that previous test case has already been completed and check can be run.
var testOverrideBeaconConfigExecuted bool

func TestConfig_OverrideBeaconConfig(t *testing.T) {
	// Ensure that param modifications are safe.
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.SlotsPerEpoch = 5
	params.OverrideBeaconConfig(cfg)
	if c := params.BeaconConfig(); c.SlotsPerEpoch != 5 {
		t.Errorf("Shardcount in BeaconConfig incorrect. Wanted %d, got %d", 5, c.SlotsPerEpoch)
	}
	testOverrideBeaconConfigExecuted = true
}

func TestConfig_OverrideBeaconConfigTestTeardown(t *testing.T) {
	if !testOverrideBeaconConfigExecuted {
		t.Skip("State leak can occur only if state mutating test has already completed")
	}
	cfg := params.BeaconConfig()
	if cfg.SlotsPerEpoch == 5 {
		t.Fatal("Parameter update has been leaked out of previous test")
	}
}

func TestConfig_DataRace(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	wg := new(sync.WaitGroup)
	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cfg := params.BeaconConfig()
			params.OverrideBeaconConfig(cfg)
		}()
		go func() uint64 {
			defer wg.Done()
			return params.BeaconConfig().MaxDeposits
		}()
	}
	wg.Wait()
}
