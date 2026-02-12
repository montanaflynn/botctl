package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/montanaflynn/botctl-go/internal/service"
	"github.com/montanaflynn/botctl-go/internal/tui"
	"github.com/spf13/cobra"
)

func init() {
	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a bot (opens dashboard by default)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// No args -> just open TUI
			if len(args) == 0 {
				tui.Version = Version
				return tui.Run()
			}

			return withService(func(svc *service.Service) error {
				name := args[0]
				bot, err := svc.GetBot(name)
				if err != nil {
					return fmt.Errorf("bot not found: %s", name)
				}

				detach, _ := cmd.Flags().GetBool("detach")
				message, _ := cmd.Flags().GetString("message")
				once, _ := cmd.Flags().GetBool("once")

				// One-shot mode (always detached, tail logs until done)
				if once {
					pid, err := svc.StartBotOnce(name, message)
					if err != nil {
						return err
					}
					fmt.Printf("  %s running once (pid %d)\n", name, pid)

					sigCh := make(chan os.Signal, 1)
					signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

					// Poll DB for log entries until process exits
					var lastSeenID int64
					for {
						select {
						case <-sigCh:
							svc.StopBot(name)
							fmt.Printf("\n  %s stopped\n", name)
							return nil
						default:
						}

						entries := svc.LogEntriesAfter(bot.ID, lastSeenID, 500)
						for _, e := range entries {
							for _, line := range svc.RenderLogEntry(e) {
								fmt.Println(line)
							}
							lastSeenID = e.ID
						}

						if running, _ := svc.IsRunning(bot.ID); !running {
							// Drain remaining entries
							entries = svc.LogEntriesAfter(bot.ID, lastSeenID, 500)
							for _, e := range entries {
								for _, line := range svc.RenderLogEntry(e) {
									fmt.Println(line)
								}
							}
							return nil
						}

						time.Sleep(1 * time.Second)
					}
				}

				// Bot already running
				if bot.Status == "running" {
					if message != "" {
						// Restart with new message
						svc.StopBot(name)
						pid, err := svc.StartBotWithMessage(name, message)
						if err != nil {
							return fmt.Errorf("restart %s: %w", name, err)
						}
						if detach {
							fmt.Printf("  %s restarted with message (pid %d)\n", name, pid)
							return nil
						}
						tui.Version = Version
						return tui.Run()
					}
					if detach {
						fmt.Printf("  %s already running\n", name)
						return nil
					}
					tui.Version = Version
					return tui.Run()
				}

				// Start the bot
				var pid int
				if message != "" {
					pid, err = svc.StartBotWithMessage(name, message)
				} else {
					pid, err = svc.StartBot(name)
				}
				if err != nil {
					return fmt.Errorf("start %s: %w", name, err)
				}

				if detach {
					fmt.Printf("  %s started (pid %d)\n", name, pid)
					return nil
				}

				tui.Version = Version
				return tui.Run()
			})
		},
	}

	startCmd.Flags().BoolP("detach", "d", false, "Run in background without opening dashboard")
	startCmd.Flags().StringP("message", "m", "", "Message to send to the bot")
	startCmd.Flags().Bool("once", false, "Run a single task and exit")
	rootCmd.AddCommand(startCmd)
}
