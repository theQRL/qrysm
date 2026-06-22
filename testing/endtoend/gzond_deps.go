package endtoend

// This file contains the dependencies required for github.com/theQRL/go-qrl/cmd/gqrl.
// Having these dependencies listed here helps go mod understand that these dependencies are
// necessary for end to end tests since we build go-qrl binary for this test.
import (
	_ "github.com/theQRL/go-qrl/accounts"          // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/accounts/keystore" // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/cmd/utils"         // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/common"            // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/console"           // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/log"               // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/metrics"           // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/node"              // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/qrl"               // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/qrl/downloader"    // Required for go-qrl e2e.
	_ "github.com/theQRL/go-qrl/qrlclient"         // Required for go-qrl e2e.
)
