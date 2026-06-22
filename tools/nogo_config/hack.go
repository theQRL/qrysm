package main

import (
	_ "github.com/golang/glog"          // Hack to keep go mod tidy happy. This dep is needed by tooling.
	_ "github.com/jedisct1/go-minisign" // Hack to keep go mod tidy happy. This dep is needed by Bazel tooling.
	_ "github.com/naoina/toml"          // Hack to keep go mod tidy happy. This dep is needed by Bazel tooling.
	_ "honnef.co/go/tools/staticcheck"  // Hack to keep go mod tidy happy. This dep is needed by bazel tooling.
)
