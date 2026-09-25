package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adevmachine/cli/internal/config"
	"github.com/adevmachine/cli/internal/packages"
	agentskills "github.com/adevmachine/cli/internal/skills"
	"github.com/spf13/cobra"
)

const officialSkillsPackage = "devmachine-skills"

func newSkillsCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install package-contributed Agent Skills on this computer",
		Long:  "Local only: these commands never connect to a configured machine. `devmachine sync` installs skills in remote workspaces.",
	}
	cmd.AddCommand(
		newSkillsAddCmd(opts),
		newSkillsListCmd(opts),
		newSkillsUpdateCmd(opts),
		newSkillsRemoveCmd(opts),
	)
	return cmd
}

func newSkillsAddCmd(opts *options) *cobra.Command {
	var requested string
	var agentNames []string
	var yes bool
	c := &cobra.Command{
		Use:   "add",
		Short: "Install the official skills or a local package's skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			source, err := skillSource(cmd.Context(), dir, requested)
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			agents, err := chooseAgents(cmd, home, agentNames, yes)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Install skills from "+source.ID+"?")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			result, err := (agentskills.Installer{Home: home}).Install(source, agents)
			if err != nil {
				return err
			}
			return printSkillResult(cmd, opts, "installed", result)
		},
	}
	c.Flags().StringVar(&requested, "package", "", "install skills from this local package instead of the official package")
	c.Flags().StringSliceVar(&agentNames, "agent", nil, "enable a harness adapter: claude, codex, pi or opencode (repeatable)")
	c.Flags().BoolVar(&yes, "yes", false, "do not ask")
	return c
}

func newSkillsListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List skills managed on this computer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			installed, err := (agentskills.Installer{Home: home}).List()
			if err != nil {
				return err
			}
			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), installed)
			}
			if len(installed) == 0 {
				cmd.Println("No skills are managed by Devmachine.")
				return nil
			}
			for _, record := range installed {
				cmd.Printf("%s\n  skills: %s\n  agents: %s\n", record.Source, strings.Join(record.Skills, ", "), joinAgents(record.Agents))
			}
			return nil
		},
	}
}

func newSkillsUpdateCmd(opts *options) *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "update",
		Short: "Refresh every managed skill from its current package source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _, err := config.Dir(opts.configDir)
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			installer := agentskills.Installer{Home: home}
			records, err := installer.List()
			if err != nil {
				return err
			}
			if len(records) == 0 {
				cmd.Println("No skills are managed by Devmachine.")
				return nil
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Update %d skill source(s)?", len(records)))
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			results := make([]agentskills.InstallResult, 0, len(records))
			for _, record := range records {
				source, err := skillSourceID(cmd.Context(), dir, record.Source)
				if err != nil {
					return err
				}
				result, err := installer.Install(source, record.Agents)
				if err != nil {
					return err
				}
				results = append(results, result)
			}
			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), results)
			}
			for _, result := range results {
				if err := printSkillResult(cmd, opts, "updated", result); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "do not ask")
	return c
}

func newSkillsRemoveCmd(opts *options) *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove one skill managed by Devmachine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			installer := agentskills.Installer{Home: home}
			records, err := installer.List()
			if err != nil {
				return err
			}
			source := ""
			for _, record := range records {
				if slices.Contains(record.Skills, name) {
					if source != "" {
						return fmt.Errorf("skill %q appears in more than one ownership record", name)
					}
					source = record.Source
				}
			}
			if source == "" {
				return fmt.Errorf("skill %q is not managed by Devmachine", name)
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Remove skill "+name+" from this computer?")
				if err != nil {
					return err
				}
				if !ok {
					return errDeclined
				}
			}
			result, err := installer.Remove(source, []string{name})
			if err != nil {
				return err
			}
			return printSkillResult(cmd, opts, "removed", result)
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "do not ask")
	return c
}

func skillSource(ctx context.Context, configDir, requested string) (agentskills.Source, error) {
	cfg, err := config.Load(configDir)
	if err != nil {
		return agentskills.Source{}, err
	}
	if requested == "" && cfg.Packages == "" {
		return agentskills.Source{}, errors.New("no package release is pinned; run `devmachine setup` or set `packages:` before `devmachine skills add`")
	}
	store, err := openStore(ctx, configDir, cfg.Packages)
	if err != nil {
		return agentskills.Source{}, err
	}
	if requested == "" {
		found, err := store.GetRelease(officialSkillsPackage)
		if err != nil {
			return agentskills.Source{}, err
		}
		return sourceFromFound(found, packages.SourceRelease+":"+officialSkillsPackage)
	}
	found, err := store.Get(requested)
	if err != nil {
		return agentskills.Source{}, err
	}
	if found.Source != packages.SourceLocal {
		return agentskills.Source{}, fmt.Errorf("--package %s requires a local package under %s", requested, packages.LocalDir(configDir))
	}
	return sourceFromFound(found, packages.SourceLocal+":"+requested)
}

func skillSourceID(ctx context.Context, configDir, id string) (agentskills.Source, error) {
	kind, name, ok := strings.Cut(id, ":")
	if !ok || name == "" {
		return agentskills.Source{}, fmt.Errorf("skill source %q is not understood", id)
	}
	if kind == packages.SourceRelease {
		if name != officialSkillsPackage {
			return agentskills.Source{}, fmt.Errorf("release skill source %q is not supported", id)
		}
		return skillSource(ctx, configDir, "")
	}
	if kind == packages.SourceLocal {
		return skillSource(ctx, configDir, name)
	}
	return agentskills.Source{}, fmt.Errorf("skill source %q is not understood", id)
}

func sourceFromFound(found packages.Found, id string) (agentskills.Source, error) {
	if found.Manifest.Skills == nil {
		return agentskills.Source{}, fmt.Errorf("package %s does not contribute skills", found.Manifest.Name)
	}
	return agentskills.Source{ID: id, Root: filepath.Join(found.Manifest.Path, found.Manifest.Skills.Path)}, nil
}

func chooseAgents(cmd *cobra.Command, home string, names []string, yes bool) ([]agentskills.Agent, error) {
	if len(names) > 0 {
		return parseAgents(names)
	}
	detected := detectAgents(home)
	if len(detected) == 0 {
		return nil, errors.New("no supported agent harness was detected; pass --agent claude, codex, pi or opencode")
	}
	if yes {
		return detected, nil
	}
	var chosen []agentskills.Agent
	for _, agent := range detected {
		ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Enable skills for "+string(agent)+"?")
		if err != nil {
			return nil, err
		}
		if ok {
			chosen = append(chosen, agent)
		}
	}
	if len(chosen) == 0 {
		return nil, errDeclined
	}
	return chosen, nil
}

func parseAgents(names []string) ([]agentskills.Agent, error) {
	agents := make([]agentskills.Agent, 0, len(names))
	for _, name := range names {
		agents = append(agents, agentskills.Agent(name))
	}
	// Installer owns validation and canonical ordering; this keeps a single
	// source of truth for accepted names.
	return agents, nil
}

func detectAgents(home string) []agentskills.Agent {
	candidates := []struct {
		agent agentskills.Agent
		path  string
	}{
		{agentskills.AgentClaude, filepath.Join(home, ".claude")},
		{agentskills.AgentCodex, filepath.Join(home, ".codex")},
		{agentskills.AgentPi, filepath.Join(home, ".pi")},
		{agentskills.AgentOpenCode, filepath.Join(home, ".config", "opencode")},
	}
	var found []agentskills.Agent
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate.path); err == nil && info.IsDir() {
			found = append(found, candidate.agent)
		}
	}
	return found
}

func printSkillResult(cmd *cobra.Command, opts *options, verb string, result agentskills.InstallResult) error {
	if opts.format == formatJSON {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	changed := "no filesystem changes"
	if result.Changed == 1 {
		changed = "1 filesystem change"
	} else if result.Changed > 1 {
		changed = fmt.Sprintf("%d filesystem changes", result.Changed)
	}
	cmd.Printf("%s %s from %s for %s (%s)\n", verb, strings.Join(result.Skills, ", "), result.Source, joinAgents(result.Agents), changed)
	return nil
}

func joinAgents(agents []agentskills.Agent) string {
	names := make([]string, len(agents))
	for index, agent := range agents {
		names[index] = string(agent)
	}
	return strings.Join(names, ", ")
}
