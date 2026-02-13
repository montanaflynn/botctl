package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl/internal/update"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update botctl to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Checking for updates...")

			latest, err := update.CheckLatest()
			if err != nil {
				return fmt.Errorf("check for updates: %w", err)
			}

			if !update.IsNewer(rawVersion, latest) {
				fmt.Printf("Already up to date (%s)\n", rawVersion)
				return nil
			}

			fmt.Printf("Updating %s → %s...\n", rawVersion, latest)
			version, err := update.SelfUpdate()
			if err != nil {
				return err
			}

			fmt.Printf("Updated to %s\n", version)
			return nil
		},
	})
}
