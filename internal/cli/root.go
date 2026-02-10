package cli

import (
	"fmt"
	"os"

	"github.com/montanaflynn/botctl-go/internal/discovery"
	"github.com/montanaflynn/botctl-go/internal/paths"
	"github.com/montanaflynn/botctl-go/internal/tui"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "botctl",
	Short: "`botctl` - bot process manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Println("botctl", Version)
			return nil
		}
		tui.Version = Version
		return tui.Run()
	},
}

func init() {
	rootCmd.Flags().Bool("version", false, "Print version")
}

func findBot(name string) *discovery.Bot {
	bots, _ := discovery.DiscoverBots()
	for _, b := range bots {
		if b.Name == name {
			return &b
		}
	}
	return nil
}

// Execute runs the root command.
func Execute() {
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
