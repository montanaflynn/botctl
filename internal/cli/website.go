package cli

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/montanaflynn/botctl/internal/website"
	"github.com/spf13/cobra"
)

func init() {
	ws := &cobra.Command{
		Use:   "website",
		Short: "Build or serve the botctl website",
	}

	// --- build ---
	build := &cobra.Command{
		Use:   "build",
		Short: "Build website to dist with CLI help injected",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				output = filepath.Join(dir, "dist")
			}

			helpText := captureHelpText()
			if err := website.Build(dir, output, helpText); err != nil {
				return err
			}
			fmt.Printf("Built %s\n", output)
			return nil
		},
	}
	build.Flags().String("dir", "./website", "Source website directory")
	build.Flags().StringP("output", "o", "", "Output directory (default <dir>/dist)")

	// --- serve ---
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve the website locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			port, _ := cmd.Flags().GetInt("port")

			helpText := captureHelpText()
			return website.Serve(dir, port, helpText)
		},
	}
	serve.Flags().String("dir", "./website", "Source website directory")
	serve.Flags().Int("port", 3000, "Port to serve on")

	ws.AddCommand(build, serve)
	rootCmd.AddCommand(ws)
}

// captureHelpText renders the root command's --help output to a string.
func captureHelpText() string {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.Help()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	return buf.String()
}
