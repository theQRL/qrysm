package params

// InteropConfig provides a generic config suitable for interop testing.
func InteropConfig() *BeaconChainConfig {
	c := MainnetConfig().Copy()

	// Qrysm constants.
	c.ConfigName = InteropName
	c.GenesisForkVersion = []byte{0, 0, 0, 235}

	c.InitializeForkSchedule()
	return c
}
