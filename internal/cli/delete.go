package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/montanaflynn/botctl/pkg/service"
	"github.com/spf13/cobra"
)

func init() {
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a bot and its data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				name := args[0]
				bot, err := svc.GetBot(name)
				if err != nil {
					return fmt.Errorf("bot not found: %s", name)
				}

				yes, _ := cmd.Flags().GetBool("yes")
				if !yes {
					fmt.Printf("  Bot:   %s\n", bot.Name)
					fmt.Printf("  Path:  %s\n", bot.Path)
					fmt.Printf("  Runs:  %d\n", bot.Stats.Runs)
					fmt.Printf("  Cost:  $%.2f\n", bot.Stats.TotalCost)
					fmt.Printf("  Turns: %d\n", bot.Stats.TotalTurns)
					if bot.Status == "running" {
						fmt.Printf("  Status: running (will be stopped)\n")
					}
					fmt.Printf("\nDelete this bot and all its data? [y/N] ")
					scanner := bufio.NewScanner(os.Stdin)
					if !scanner.Scan() || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(scanner.Text())), "y") {
						fmt.Println("  cancelled")
						return nil
					}
				}

				if err := svc.DeleteBot(name); err != nil {
					return err
				}
				fmt.Printf("  %s deleted\n", name)
				return nil
			})
		},
	}

	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}
