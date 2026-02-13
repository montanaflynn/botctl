package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl/pkg/service"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "pause [name]",
		Short: "Pause a running or sleeping bot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				name := args[0]
				err := svc.PauseBot(name)
				if err == service.ErrBotNotFound {
					return fmt.Errorf("bot not found: %s", name)
				}
				if err == service.ErrNotActive {
					fmt.Printf("  %s is not active\n", name)
					return nil
				}
				if err != nil {
					return err
				}
				fmt.Printf("  %s pausing...\n", name)
				return nil
			})
		},
	})
}
