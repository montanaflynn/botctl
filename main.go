package main

import (
	"github.com/montanaflynn/botctl/internal/cli"

	// Register agent backends
	_ "github.com/montanaflynn/botctl/pkg/backend/claude"
	_ "github.com/montanaflynn/botctl/pkg/backend/opencode"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	cli.Execute(version)
}
