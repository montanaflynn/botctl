package cli

import (
	"github.com/montanaflynn/botctl-go/internal/harness"
	"github.com/spf13/cobra"
)

func init() {
	harnessCmd := &cobra.Command{
		Use:    "harness <bot_dir>",
		Short:  "Run the bot harness (internal)",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			once, _ := cmd.Flags().GetBool("once")
			message, _ := cmd.Flags().GetString("message")
			return harness.Run(args[0], once, message)
		},
	}
	harnessCmd.Flags().Bool("once", false, "Run a single task and exit")
	harnessCmd.Flags().String("message", "", "Message to append to the prompt")
	rootCmd.AddCommand(harnessCmd)
}
