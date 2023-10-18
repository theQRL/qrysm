package endtoend

// This file contains the dependencies required for github.com/theQRL/go-zond/cmd/gzond.
// Having these dependencies listed here helps go mod understand that these dependencies are
// necessary for end to end tests since we build go-ethereum binary for this test.
import (
	_ "github.com/theQRL/go-zond/accounts"          // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/accounts/keystore" // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/cmd/utils"         // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/common"            // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/console"           // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/les"               // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/log"               // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/metrics"           // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/node"              // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/zond"              // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/zond/downloader"   // Required for go-ethereum e2e.
	_ "github.com/theQRL/go-zond/zondclient"        // Required for go-ethereum e2e.
)
