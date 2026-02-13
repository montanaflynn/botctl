package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl/pkg/service"
	"github.com/spf13/cobra"
)

func init() {
	playCmd := &cobra.Command{
		Use:   "play [name]",
		Short: "Resume a paused bot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				name := args[0]
				turns, _ := cmd.Flags().GetInt("turns")
				if turns <= 0 {
					turns = 50
				}

				pid, err := svc.PlayBot(name, turns)
				if err == service.ErrBotNotFound {
					return fmt.Errorf("bot not found: %s", name)
				}
				if err == service.ErrNotPaused {
					return fmt.Errorf("%s is not paused (use 'botctl start' to start a stopped bot)", name)
				}
				if err != nil {
					return err
				}
				fmt.Printf("  %s playing (pid %d, %d turns)\n", name, pid, turns)
				return nil
			})
		},
	}

	playCmd.Flags().IntP("turns", "t", 50, "Number of turns for the resumed session")
	rootCmd.AddCommand(playCmd)
}
