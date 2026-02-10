package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/discovery"
	"github.com/montanaflynn/botctl-go/internal/process"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List discovered bots",
		RunE: func(cmd *cobra.Command, args []string) error {
			database, err := db.Open()
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			bots, errors := discovery.DiscoverBots()
			for _, e := range errors {
				fmt.Printf("  warning: %s\n", e)
			}
			if len(bots) == 0 {
				fmt.Println("No bots found")
				return nil
			}
			for _, bot := range bots {
				running, pid := process.IsRunning(bot.ID, database)
				status := "stopped"
				if running {
					status = fmt.Sprintf("running (pid %d)", pid)
				}
				fmt.Printf("  %-20s %s\n", bot.Name, status)
			}
			return nil
		},
	})
}
