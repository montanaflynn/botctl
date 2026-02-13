package cli

import (
	"fmt"

	"github.com/montanaflynn/botctl/pkg/create"
	"github.com/spf13/cobra"
)

func init() {
	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new bot via Claude",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := create.Params{}

			if len(args) > 0 {
				p.Name = args[0]
			}
			if d, _ := cmd.Flags().GetString("description"); d != "" {
				p.Description = d
			}
			if i, _ := cmd.Flags().GetInt("interval"); i > 0 {
				p.Interval = i
			}
			if mt, _ := cmd.Flags().GetInt("max-turns"); mt > 0 {
				p.MaxTurns = mt
			}

			// If name or description missing, prompt interactively
			if p.Name == "" || p.Description == "" {
				var err error
				p, err = create.InteractiveParams(p)
				if err != nil {
					return err
				}
			}

			// Apply defaults for non-interactive fields
			if p.Interval == 0 {
				p.Interval = 300
			}
			if p.MaxTurns == 0 {
				p.MaxTurns = 20
			}

			fmt.Println("Generating BOT.md...")
			path, err := create.Run(p, nil)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s\n", path)
			return nil
		},
	}

	createCmd.Flags().StringP("description", "d", "", "Bot description")
	createCmd.Flags().IntP("interval", "i", 0, "Run interval in seconds (default 300)")
	createCmd.Flags().IntP("max-turns", "m", 0, "Max Claude turns per run (default 20)")

	rootCmd.AddCommand(createCmd)
}
