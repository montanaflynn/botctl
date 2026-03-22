package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/montanaflynn/botctl/pkg/config"
	"github.com/montanaflynn/botctl/pkg/paths"
	"github.com/montanaflynn/botctl/pkg/skills"
	"github.com/spf13/cobra"
)

func init() {
	svc := skills.NewService()

	sk := &cobra.Command{
		Use:   "skills",
		Short: "Manage skills",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List all discovered skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			botName, _ := cmd.Flags().GetString("bot")
			jsonOut, _ := cmd.Flags().GetBool("json")

			found := svc.List(botName)
			if len(found) == 0 {
				if jsonOut {
					fmt.Println("[]")
				} else {
					fmt.Println("No skills found")
				}
				return nil
			}

			if jsonOut {
				type entry struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Source      string `json:"source"`
					Path        string `json:"path"`
				}
				out := make([]entry, len(found))
				for i, s := range found {
					out[i] = entry{s.Name, s.Description, s.Source, s.Path}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			home, _ := os.UserHomeDir()

			// Truncate description to first sentence or maxDescW chars.
			const maxDescW = 48
			truncate := func(s string) string {
				// Use first sentence if it fits.
				if i := strings.Index(s, ". "); i > 0 && i <= maxDescW {
					return s[:i+1]
				}
				if len(s) <= maxDescW {
					return s
				}
				return s[:maxDescW-3] + "..."
			}

			// Build display rows and measure column widths.
			type row struct{ name, desc, source string }
			rows := make([]row, len(found))
			nameW := len("NAME")
			descW := len("DESCRIPTION")
			sourceW := len("SOURCE")
			for i, s := range found {
				source := s.Source
				if home != "" && strings.HasPrefix(source, home) {
					source = "~" + source[len(home):]
				}
				r := row{s.Name, truncate(s.Description), source}
				if len(r.name) > nameW {
					nameW = len(r.name)
				}
				if len(r.desc) > descW {
					descW = len(r.desc)
				}
				if len(r.source) > sourceW {
					sourceW = len(r.source)
				}
				rows[i] = r
			}

			fmt.Printf("%-*s  %-*s  %s\n", nameW, "NAME", descW, "DESCRIPTION", "SOURCE")
			for _, r := range rows {
				fmt.Printf("%-*s  %-*s  %s\n", nameW, r.name, descW, r.desc, r.source)
			}
			return nil
		},
	}
	list.Flags().String("bot", "", "Only include bot-specific skills from this bot")
	list.Flags().Bool("json", false, "Output as JSON")

	search := &cobra.Command{
		Use:   "search <query>",
		Short: "Search skills.sh for community skills",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			limit, _ := cmd.Flags().GetInt("limit")
			jsonOut, _ := cmd.Flags().GetBool("json")

			results, err := svc.Search(query, limit)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				if jsonOut {
					fmt.Println("[]")
				} else {
					fmt.Printf("No skills found for %q\n", query)
				}
				return nil
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			// Build display rows and measure column widths.
			type row struct{ name, source, installs string }
			rows := make([]row, len(results))
			nameW := len("NAME")
			sourceW := len("SOURCE")
			for i, s := range results {
				r := row{s.Name, s.Source, formatInstalls(s.Installs)}
				if len(r.name) > nameW {
					nameW = len(r.name)
				}
				if len(r.source) > sourceW {
					sourceW = len(r.source)
				}
				rows[i] = r
			}

			fmt.Printf("%-*s  %-*s  %s\n", nameW, "NAME", sourceW, "SOURCE", "INSTALLS")
			for _, r := range rows {
				fmt.Printf("%-*s  %-*s  %s\n", nameW, r.name, sourceW, r.source, r.installs)
			}
			return nil
		},
	}
	search.Flags().IntP("limit", "n", 10, "Max results to return")
	search.Flags().Bool("json", false, "Output as JSON")

	add := &cobra.Command{
		Use:   "add <owner/repo>",
		Short: "Install skills from a GitHub repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			skillFilter, _ := cmd.Flags().GetString("skill")
			all, _ := cmd.Flags().GetBool("all")
			botName, _ := cmd.Flags().GetString("bot")
			global, _ := cmd.Flags().GetBool("global")

			if skillFilter == "" && !all {
				return fmt.Errorf("specify a skill with --skill <name>, or use --all to install every skill from the repo")
			}

			// Resolve destination directory.
			var destDir string
			switch {
			case botName != "":
				botDir := filepath.Join(paths.BotsDir(), botName)
				cfg, err := config.FromMD(filepath.Join(botDir, "BOT.md"))
				if err != nil {
					return fmt.Errorf("load bot %s: %w", botName, err)
				}
				if cfg.SkillsDir == "" {
					return fmt.Errorf("bot %s has no skills_dir configured in BOT.md", botName)
				}
				destDir = filepath.Join(botDir, cfg.SkillsDir)
				if abs, err := filepath.Abs(destDir); err == nil {
					destDir = abs
				}
			case global:
				destDir = paths.AgentsSkillsDir()
			default:
				destDir = paths.GlobalSkillsDir()
			}

			fmt.Printf("Installing skills from %s...\n", source)
			installed, err := svc.Install(source, skills.InstallOpts{
				SkillFilter: skillFilter,
				DestDir:     destDir,
			})
			if err != nil {
				return err
			}

			home, _ := os.UserHomeDir()
			displayDir := destDir
			if home != "" && strings.HasPrefix(displayDir, home) {
				displayDir = "~" + displayDir[len(home):]
			}

			for _, name := range installed {
				fmt.Printf("  installed %s → %s\n", name, displayDir)
			}
			fmt.Printf("%d skill(s) installed\n", len(installed))
			return nil
		},
	}
	add.Flags().String("skill", "", "Install only this skill from the repo")
	add.Flags().Bool("all", false, "Install all skills from the repo")
	add.Flags().String("bot", "", "Install to a bot's skills_dir")
	add.Flags().Bool("global", false, "Install to ~/.agents/skills/ (cross-agent)")

	view := &cobra.Command{
		Use:   "view <name>",
		Short: "View a skill's SKILL.md and list its files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			repo, _ := cmd.Flags().GetString("repo")
			jsonOut, _ := cmd.Flags().GetBool("json")

			if !jsonOut && repo != "" {
				source := strings.TrimPrefix(strings.TrimPrefix(repo, "https://"), "github.com/")
				fmt.Printf("Fetching %s from %s...\n", name, source)
			}

			result, err := svc.View(name, repo)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Print(result.Content)
			if len(result.Content) > 0 && result.Content[len(result.Content)-1] != '\n' {
				fmt.Println()
			}
			if len(result.Files) > 0 {
				fmt.Printf("\nFiles:\n")
				for _, f := range result.Files {
					fmt.Printf("  %s\n", f)
				}
			}
			return nil
		},
	}
	view.Flags().String("repo", "", "GitHub repo to fetch from (e.g. github.com/owner/repo)")
	view.Flags().Bool("json", false, "Output as JSON")

	remove := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			removed, err := svc.Remove(name)
			if err != nil {
				return err
			}

			home, _ := os.UserHomeDir()
			for _, dir := range removed {
				display := dir
				if home != "" && strings.HasPrefix(display, home) {
					display = "~" + display[len(home):]
				}
				fmt.Printf("Removed %s from %s\n", name, display)
			}
			return nil
		},
	}

	sk.AddCommand(list, search, add, view, remove)
	rootCmd.AddCommand(sk)
}

func formatInstalls(n int) string {
	if n <= 0 {
		return ""
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
