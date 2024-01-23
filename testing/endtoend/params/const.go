package params

const (
	// Every EL component has an offset that manages which port it is assigned. The miner always gets offset=0.
	MinerComponentOffset = 0
	Eth1StaticFilesPath  = "/testing/endtoend/static-files/eth1"
	minerKeyFilename     = "UTC--2024-01-04T08-08-35.961423000Z--205547ba6232eec096770f7161d57dea54fd13d0"
	baseELHost           = "127.0.0.1"
	baseELScheme         = "http"
	// DepositGasLimit is the gas limit used for all deposit transactions. The exact value probably isn't important
	// since these are the only transactions in the e2e run.
	DepositGasLimit = 4000000
	// SpamTxGasLimit is used for the spam transactions (to/from miner address)
	// which WaitForBlocks generates in order to advance the EL chain.
	SpamTxGasLimit = 21000
)
