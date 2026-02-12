package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl-go/internal/service"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List discovered bots",
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
					fmt.Printf("  %-20s %s\n", bot.Name, status)
				}
				return nil
			})
		},
	})
}
