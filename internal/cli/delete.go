package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/process"
	"github.com/spf13/cobra"
)

func init() {
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a bot and its data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bot := findBot(args[0])
			if bot == nil {
				return fmt.Errorf("bot not found: %s", args[0])
			}

			database, err := db.Open()
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				stats := database.GetBotStats(bot.ID)
				running, _ := process.IsRunning(bot.ID, database)

				fmt.Printf("  Bot:   %s\n", bot.Name)
				fmt.Printf("  Path:  %s\n", bot.Path)
				fmt.Printf("  Runs:  %d\n", stats.Runs)
				fmt.Printf("  Cost:  $%.2f\n", stats.TotalCost)
				fmt.Printf("  Turns: %d\n", stats.TotalTurns)
				if running {
					fmt.Printf("  Status: running (will be stopped)\n")
				}
				fmt.Printf("\nDelete this bot and all its data? [y/N] ")
				scanner := bufio.NewScanner(os.Stdin)
				if !scanner.Scan() || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(scanner.Text())), "y") {
					fmt.Println("  cancelled")
					return nil
				}
			}

			// Stop if running
			if running, _ := process.IsRunning(bot.ID, database); running {
				process.StopBot(bot.ID, database)
				fmt.Printf("  %s stopped\n", bot.Name)
			}

			// Remove DB records
			if err := database.DeleteBotData(bot.ID); err != nil {
				return fmt.Errorf("delete db data: %w", err)
			}

			// Remove bot directory
			if err := os.RemoveAll(bot.Path); err != nil {
				return fmt.Errorf("delete bot directory: %w", err)
			}

			fmt.Printf("  %s deleted\n", bot.Name)
			return nil
		},
	}

	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}
