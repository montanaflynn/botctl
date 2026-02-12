package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl-go/internal/service"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stop [name]",
		Short: "Stop bot(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				if len(args) > 0 {
					err := svc.StopBot(args[0])
					if err == service.ErrBotNotFound {
						return fmt.Errorf("bot not found: %s", args[0])
					}
					if err == service.ErrNotRunning {
						fmt.Printf("  %s not running\n", args[0])
						return nil
					}
					if err != nil {
						return err
					}
					fmt.Printf("  %s stopped\n", args[0])
					return nil
				}

				// Stop all bots
				bots, _ := svc.ListBots("")
				for _, bot := range bots {
					err := svc.StopBot(bot.Name)
					if err == service.ErrNotRunning {
						fmt.Printf("  %s not running\n", bot.Name)
					} else if err != nil {
						fmt.Printf("  %s error: %v\n", bot.Name, err)
					} else {
						fmt.Printf("  %s stopped\n", bot.Name)
					}
				}
				return nil
			})
		},
	})
}
