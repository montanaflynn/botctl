package main

import "github.com/montanaflynn/botctl/internal/cli"

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	cli.Execute(version)
}
