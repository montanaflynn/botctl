package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/montanaflynn/botctl/internal/service"
	"github.com/spf13/cobra"
)

func init() {
	logsCmd := &cobra.Command{
		Use:   "logs [name]",
		Short: "View bot logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lines, _ := cmd.Flags().GetInt("lines")
			follow, _ := cmd.Flags().GetBool("follow")

			return withService(func(svc *service.Service) error {
				var targets []service.BotInfo
				if len(args) > 0 {
					bot, err := svc.GetBot(args[0])
					if err != nil {
						return fmt.Errorf("bot not found: %s", args[0])
					}
					targets = []service.BotInfo{bot}
				} else {
					targets, _ = svc.ListBots("")
				}

				if !follow {
					for _, bot := range targets {
						recent := svc.RecentLogs(bot.ID, lines)
						if len(recent) == 0 {
							continue
						}
						if len(targets) > 1 {
							fmt.Printf("==> %s <==\n", bot.Name)
						}
						fmt.Print(strings.Join(recent, "\n"))
						fmt.Println()
						if len(targets) > 1 {
							fmt.Println()
						}
					}
					return nil
				}

				// Follow mode -- poll DB for new entries
				if len(targets) == 0 {
					fmt.Println("No bots found")
					return nil
				}

				type tracker struct {
					bot        service.BotInfo
					lastSeenID int64
				}
				var trackers []tracker
				for _, bot := range targets {
					entries := svc.RecentLogEntries(bot.ID, lines)
					var lastID int64
					if len(entries) > 0 {
						if len(targets) > 1 {
							fmt.Printf("==> %s <==\n", bot.Name)
						}
						for _, e := range entries {
							for _, line := range svc.RenderLogEntry(e) {
								fmt.Println(line)
							}
							lastID = e.ID
						}
					}
					trackers = append(trackers, tracker{bot: bot, lastSeenID: lastID})
				}

				// Poll loop
				for {
					time.Sleep(1 * time.Second)
					for i := range trackers {
						t := &trackers[i]
						entries := svc.LogEntriesAfter(t.bot.ID, t.lastSeenID, 500)
						if len(entries) == 0 {
							continue
						}
						if len(targets) > 1 {
							fmt.Printf("==> %s <==\n", t.bot.Name)
						}
						for _, e := range entries {
							for _, line := range svc.RenderLogEntry(e) {
								fmt.Println(line)
							}
							t.lastSeenID = e.ID
						}
					}
				}
			})
		},
	}

	logsCmd.Flags().IntP("lines", "n", 20, "Number of lines to show")
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}
