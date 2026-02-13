package cli

import (
	"fmt"
	"os"

	"github.com/montanaflynn/botctl/pkg/db"
	"github.com/montanaflynn/botctl/pkg/paths"
	"github.com/montanaflynn/botctl/pkg/service"
	"github.com/montanaflynn/botctl/internal/tui"
	"github.com/montanaflynn/botctl/internal/update"
	"github.com/montanaflynn/botctl/internal/web"
	"github.com/spf13/cobra"
)

// rawVersion holds the semver without commit info, used for update checks.
var rawVersion string

var rootCmd = &cobra.Command{
	Use:   "botctl",
	Short: "`botctl` - bot process manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		if webUI, _ := cmd.Flags().GetBool("web-ui"); webUI {
			port, _ := cmd.Flags().GetInt("port")
			return web.Serve(port)
		}
		tui.Version = cmd.Root().Version
		return tui.Run()
	},
}

func init() {
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
func Execute(version string) {
	rawVersion = version

	// Build display version with commit hash
	display := version
	if commit, dirty := update.CommitInfo(); commit != "" {
		if dirty {
			commit += "-dirty"
		}
		display += " (" + commit + ")"
	}
	rootCmd.Version = display

	// Background update check
	updateCh := make(chan string, 1)
	go func() {
		if latest, err := update.CheckLatest(); err == nil && update.IsNewer(version, latest) {
			updateCh <- latest
		}
		close(updateCh)
	}()

	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

	// Print update notice if check completed in time
	select {
	case latest := <-updateCh:
		if latest != "" {
			fmt.Fprintf(os.Stderr, "\nUpdate available: %s → %s\nRun `botctl update` to update\n", version, latest)
		}
	default:
	}
}
