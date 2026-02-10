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
		Use:   "status",
		Short: "Detailed status of all bots",
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
				fmt.Printf("  %s\n", bot.Name)
				fmt.Printf("    status:   %s\n", status)
				fmt.Printf("    interval: %ds\n", bot.Config.IntervalSeconds)
				if bot.Config.MaxTurns > 0 {
					fmt.Printf("    max_turns: %d\n", bot.Config.MaxTurns)
				}

				stats := database.GetBotStats(bot.ID)
				if stats.Runs > 0 {
					fmt.Printf("    runs:     %d ($%.4f total, %d turns)\n", stats.Runs, stats.TotalCost, stats.TotalTurns)
					if stats.LastRun != "" {
						fmt.Printf("    last at:  %s\n", stats.LastRun)
					}
				}

				fmt.Printf("    path:     %s\n", bot.Path)
				fmt.Println()
			}
			return nil
		},
	})
}
