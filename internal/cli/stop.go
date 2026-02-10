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
		Use:   "stop [name]",
		Short: "Stop bot(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			database, err := db.Open()
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			var targets []discovery.Bot

			if len(args) > 0 {
				bot := findBot(args[0])
				if bot == nil {
					return fmt.Errorf("bot not found: %s", args[0])
				}
				targets = []discovery.Bot{*bot}
			} else {
				targets, _ = discovery.DiscoverBots()
			}

			for _, bot := range targets {
				if process.StopBot(bot.ID, database) {
					fmt.Printf("  %s stopped\n", bot.Name)
				} else {
					fmt.Printf("  %s not running\n", bot.Name)
				}
			}
			return nil
		},
	})
}
