package cli

import (
	"fmt"
	"os"

	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/paths"
	"github.com/montanaflynn/botctl-go/internal/service"
	"github.com/montanaflynn/botctl-go/internal/tui"
	"github.com/montanaflynn/botctl-go/internal/web"
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
		if webUI, _ := cmd.Flags().GetBool("web-ui"); webUI {
			port, _ := cmd.Flags().GetInt("port")
			return web.Serve(port)
		}
		tui.Version = Version
		return tui.Run()
	},
}

func init() {
	rootCmd.Flags().Bool("version", false, "Print version")
	rootCmd.Flags().Bool("web-ui", false, "Start web dashboard instead of TUI")
	rootCmd.Flags().Int("port", 4444, "Port for the web dashboard")
}

// withService opens the database, creates a service, and passes it to fn.
func withService(fn func(*service.Service) error) error {
	database, err := db.Open()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()
	return fn(service.New(database))
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
