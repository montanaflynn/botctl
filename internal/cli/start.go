package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/logs"
	"github.com/montanaflynn/botctl-go/internal/process"
	"github.com/montanaflynn/botctl-go/internal/tui"
	"github.com/spf13/cobra"
)

func init() {
	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a bot (opens dashboard by default)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// No args → just open TUI
			if len(args) == 0 {
				tui.Version = Version
				return tui.Run()
			}

			bot := findBot(args[0])
			if bot == nil {
				return fmt.Errorf("bot not found: %s", args[0])
			}

			database, err := db.Open()
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			detach, _ := cmd.Flags().GetBool("detach")
			message, _ := cmd.Flags().GetString("message")
			once, _ := cmd.Flags().GetBool("once")

			running, _ := process.IsRunning(bot.ID, database)

			// One-shot mode (always detached, tail logs until done)
			if once {
				if running {
					process.StopBot(bot.ID, database)
				}
				pid, err := process.StartBotOnce(bot.Name, bot.Path, bot.Config, database, message)
				if err != nil {
					return err
				}
				fmt.Printf("  %s running once (pid %d)\n", bot.Name, pid)

				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

				// Poll DB for log entries until process exits
				var lastSeenID int64
				for {
					select {
					case <-sigCh:
						process.StopBot(bot.ID, database)
						fmt.Printf("\n  %s stopped\n", bot.Name)
						return nil
					default:
					}

					entries := database.LogEntriesAfter(bot.ID, lastSeenID, 500)
					for _, e := range entries {
						for _, line := range logs.RenderEntry(e) {
							fmt.Println(line)
						}
						lastSeenID = e.ID
					}

					running, _ := process.IsRunning(bot.ID, database)
					if !running {
						// Drain remaining entries
						entries = database.LogEntriesAfter(bot.ID, lastSeenID, 500)
						for _, e := range entries {
							for _, line := range logs.RenderEntry(e) {
								fmt.Println(line)
							}
						}
						return nil
					}

					time.Sleep(1 * time.Second)
				}
			}

			// Bot already running
			if running {
				if message != "" {
					// Restart with new message
					process.StopBot(bot.ID, database)
					pid, err := process.StartBotWithMessage(bot.Name, bot.Path, bot.Config, database, message)
					if err != nil {
						return fmt.Errorf("restart %s: %w", bot.Name, err)
					}
					if detach {
						fmt.Printf("  %s restarted with message (pid %d)\n", bot.Name, pid)
						return nil
					}
					tui.Version = Version
					return tui.Run()
				}
				// Already running, no message — just open TUI (or noop if detached)
				if detach {
					fmt.Printf("  %s already running\n", bot.Name)
					return nil
				}
				tui.Version = Version
				return tui.Run()
			}

			// Start the bot
			var pid int
			if message != "" {
				pid, err = process.StartBotWithMessage(bot.Name, bot.Path, bot.Config, database, message)
			} else {
				pid, err = process.StartBot(bot.Name, bot.Path, bot.Config, false, database)
			}
			if err != nil {
				return fmt.Errorf("start %s: %w", bot.Name, err)
			}

			if detach {
				fmt.Printf("  %s started (pid %d)\n", bot.Name, pid)
				return nil
			}

			// Open TUI
			tui.Version = Version
			return tui.Run()
		},
	}

	startCmd.Flags().BoolP("detach", "d", false, "Run in background without opening dashboard")
	startCmd.Flags().StringP("message", "m", "", "Message to send to the bot")
	startCmd.Flags().Bool("once", false, "Run a single task and exit")
	rootCmd.AddCommand(startCmd)
}
