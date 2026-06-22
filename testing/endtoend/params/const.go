package params

const (
	// Every EL component has an offset that manages which port it is assigned. The miner always gets offset=0.
	ExecutionNodeComponentOffset = 0
	StaticFilesPath              = "/testing/endtoend/static-files/qrl"
	keyFilename                  = "UTC--2025-11-06T06-58-27.976311000Z--Qaf84bc06703edfc371a0177ac8b482622d5ad24204145f01746cb381dcd546c53b8825839cc61bfc1fc3d78bc560c7bb7a9895432e1e87435474a1bc5a2e1200"
	baseELHost                   = "127.0.0.1"
	baseELScheme                 = "http"
	// DepositGasLimit is the gas limit used for all deposit transactions. The exact value probably isn't important
	// since these are the only transactions in the e2e run.
	DepositGasLimit = 4000000
	// SpamTxGasLimit is used for the spam transactions (to/from miner address)
	// which WaitForBlocks generates in order to advance the EL chain.
	SpamTxGasLimit = 21000
)
