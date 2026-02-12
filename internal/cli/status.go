package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl-go/internal/service"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Detailed status of all bots",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				bots, errors := svc.ListBots("")
				for _, e := range errors {
					fmt.Printf("  warning: %s\n", e)
				}
				if len(bots) == 0 {
					fmt.Println("No bots found")
					return nil
				}
				for _, bot := range bots {
					status := "stopped"
					if bot.Status == "running" {
						status = fmt.Sprintf("running (pid %d)", bot.PID)
					}
					fmt.Printf("  %s\n", bot.Name)
					fmt.Printf("    status:   %s\n", status)
					if bot.Config != nil {
						fmt.Printf("    interval: %ds\n", bot.Config.IntervalSeconds)
						if bot.Config.MaxTurns > 0 {
							fmt.Printf("    max_turns: %d\n", bot.Config.MaxTurns)
						}
					}

					if bot.Stats.Runs > 0 {
						fmt.Printf("    runs:     %d ($%.4f total, %d turns)\n", bot.Stats.Runs, bot.Stats.TotalCost, bot.Stats.TotalTurns)
						if bot.Stats.LastRun != "" {
							fmt.Printf("    last at:  %s\n", bot.Stats.LastRun)
						}
					}

					fmt.Printf("    path:     %s\n", bot.Path)
					fmt.Println()
				}
				return nil
			})
		},
	})
}
