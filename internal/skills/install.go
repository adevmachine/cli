package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Agent is a supported local agent harness.
type Agent string

const (
	// AgentClaude identifies Claude Code's skill adapter.
	AgentClaude Agent = "claude"
	// AgentCodex identifies Codex's canonical skill directory.
	AgentCodex Agent = "codex"
	// AgentPi identifies Pi's canonical skill directory.
	AgentPi Agent = "pi"
	// AgentOpenCode identifies OpenCode's canonical skill directory.
	AgentOpenCode Agent = "opencode"
)

var supportedAgents = []Agent{AgentClaude, AgentCodex, AgentOpenCode, AgentPi}

// Source is one package's validated skill contribution.
type Source struct {
	ID   string
	Root string
}

// Installer owns local canonical copies, adapters and ownership records.
type Installer struct {
	Home     string
	StateDir string
}

// InstallResult reports filesystem changes made by one operation.
type InstallResult struct {
	Source  string   `json:"source"`
	Skills  []string `json:"skills"`
	Agents  []Agent  `json:"agents"`
	Changed int      `json:"changed"`
}

// Install converges all skills from source and the requested harness adapters.
func (i Installer) Install(source Source, agents []Agent) (InstallResult, error) {
	result := InstallResult{Source: source.ID}
	if i.Home == "" {
		return result, errors.New("installing skills requires a home directory")
	}
	if strings.TrimSpace(source.ID) == "" {
		return result, errors.New("installing skills requires a source ID")
	}
	found, err := Discover(source.Root)
	if err != nil {
		return result, err
	}
	result.Agents, err = normalizeAgents(agents)
	if err != nil {
		return result, err
	}
	for _, skill := range found {
		result.Skills = append(result.Skills, skill.Name)
	}

	records, err := i.records()
	if err != nil {
		return result, err
	}
	owners := skillOwners(records)
	previous, hadPrevious := recordFor(records, source.ID)
	for _, skill := range found {
		destination := i.canonical(skill.Name)
		owner := owners[skill.Name]
		if _, err := os.Lstat(destination); err == nil {
			switch {
			case owner == "":
				return result, fmt.Errorf("skill %q collides with unmanaged path %s", skill.Name, destination)
			case owner != source.ID:
				return result, fmt.Errorf("skill %q from %s is already owned by %s at %s", skill.Name, source.ID, owner, destination)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if slices.Contains(result.Agents, AgentClaude) {
			if err := i.checkClaudeLink(skill.Name, source.ID, owner, hadPrevious && slices.Contains(previous.Agents, AgentClaude)); err != nil {
				return result, err
			}
		}
	}

	for _, skill := range found {
		changed, err := replaceTreeIfDifferent(skill.Path, i.canonical(skill.Name))
		if err != nil {
			return result, err
		}
		if changed {
			result.Changed++
		}
	}

	if hadPrevious && slices.Contains(previous.Agents, AgentClaude) && !slices.Contains(result.Agents, AgentClaude) {
		for _, name := range previous.Skills {
			if err := i.removeManagedClaudeLink(name); err != nil {
				return result, err
			}
			result.Changed++
		}
	}
	if slices.Contains(result.Agents, AgentClaude) {
		for _, skill := range found {
			changed, err := i.ensureClaudeLink(skill.Name)
			if err != nil {
				return result, err
			}
			if changed {
				result.Changed++
			}
		}
	}

	recordedSkills := append([]string(nil), result.Skills...)
	if hadPrevious {
		for _, name := range previous.Skills {
			if !slices.Contains(recordedSkills, name) {
				recordedSkills = append(recordedSkills, name)
			}
		}
		sort.Strings(recordedSkills)
	}
	if err := i.writeRecord(Installed{Source: source.ID, Root: source.Root, Skills: recordedSkills, Agents: result.Agents}); err != nil {
		return result, err
	}
	return result, nil
}

// Remove deletes named skills only when sourceID owns them. No names means all
// skills owned by that source.
func (i Installer) Remove(sourceID string, names []string) (InstallResult, error) {
	result := InstallResult{Source: sourceID}
	records, err := i.records()
	if err != nil {
		return result, err
	}
	owners := skillOwners(records)
	record, ok := recordFor(records, sourceID)
	if !ok {
		for _, name := range names {
			if owner := owners[name]; owner != "" {
				return result, fmt.Errorf("skill %q is owned by %s, not %s", name, owner, sourceID)
			}
		}
		return result, fmt.Errorf("no skills are managed from %s", sourceID)
	}
	result.Agents = append([]Agent(nil), record.Agents...)
	if len(names) == 0 {
		names = append([]string(nil), record.Skills...)
	}
	sort.Strings(names)
	for _, name := range names {
		if owner := owners[name]; owner != sourceID {
			if owner == "" {
				return result, fmt.Errorf("skill %q is unmanaged", name)
			}
			return result, fmt.Errorf("skill %q is owned by %s, not %s", name, owner, sourceID)
		}
	}
	for _, name := range names {
		if slices.Contains(record.Agents, AgentClaude) {
			if err := i.removeManagedClaudeLink(name); err != nil {
				return result, err
			}
			result.Changed++
		}
		if err := os.RemoveAll(i.canonical(name)); err != nil {
			return result, err
		}
		result.Changed++
		result.Skills = append(result.Skills, name)
	}
	remaining := make([]string, 0, len(record.Skills))
	for _, name := range record.Skills {
		if !slices.Contains(names, name) {
			remaining = append(remaining, name)
		}
	}
	if len(remaining) == 0 {
		if err := i.removeRecord(sourceID); err != nil {
			return result, err
		}
	} else {
		record.Skills = remaining
		if err := i.writeRecord(record); err != nil {
			return result, err
		}
	}
	return result, nil
}

func normalizeAgents(agents []Agent) ([]Agent, error) {
	set := map[Agent]bool{}
	for _, agent := range agents {
		if !slices.Contains(supportedAgents, agent) {
			return nil, fmt.Errorf("unknown agent %q: choose claude, codex, pi or opencode", agent)
		}
		set[agent] = true
	}
	out := make([]Agent, 0, len(set))
	for _, agent := range supportedAgents {
		if set[agent] {
			out = append(out, agent)
		}
	}
	return out, nil
}

func skillOwners(records []Installed) map[string]string {
	owners := map[string]string{}
	for _, record := range records {
		for _, name := range record.Skills {
			owners[name] = record.Source
		}
	}
	return owners
}

func recordFor(records []Installed, source string) (Installed, bool) {
	for _, record := range records {
		if record.Source == source {
			return record, true
		}
	}
	return Installed{}, false
}

func (i Installer) canonical(name string) string {
	return filepath.Join(i.Home, ".agents", "skills", name)
}

func (i Installer) claudeLink(name string) string {
	return filepath.Join(i.Home, ".claude", "skills", name)
}

func (i Installer) checkClaudeLink(name, source, owner string, previouslyManaged bool) error {
	link := i.claudeLink(name)
	_, err := os.Lstat(link)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != source || !previouslyManaged {
		return fmt.Errorf("claude adapter for skill %q collides with unmanaged path %s", name, link)
	}
	return nil
}

func (i Installer) ensureClaudeLink(name string) (bool, error) {
	link := i.claudeLink(name)
	target, err := filepath.Rel(filepath.Dir(link), i.canonical(name))
	if err != nil {
		return false, err
	}
	if current, err := os.Readlink(link); err == nil && current == target {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return false, err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.Symlink(target, link); err != nil {
		return false, err
	}
	return true, nil
}

func (i Installer) removeManagedClaudeLink(name string) error {
	link := i.claudeLink(name)
	target, err := filepath.Rel(filepath.Dir(link), i.canonical(name))
	if err != nil {
		return err
	}
	current, err := os.Readlink(link)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || current != target {
		return fmt.Errorf("claude adapter for skill %q at %s is no longer managed", name, link)
	}
	return os.Remove(link)
}

func replaceTreeIfDifferent(source, destination string) (bool, error) {
	equal, err := treesEqual(source, destination)
	if err != nil {
		return false, err
	}
	if equal {
		return false, nil
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, err
	}
	stage, err := os.MkdirTemp(parent, ".skill-stage-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := copyTree(source, stage); err != nil {
		return false, err
	}
	backup, err := os.MkdirTemp(parent, ".skill-backup-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(backup) }()
	if err := os.Remove(backup); err != nil {
		return false, err
	}
	hadDestination := false
	if _, err := os.Lstat(destination); err == nil {
		hadDestination = true
		if err := os.Rename(destination, backup); err != nil {
			return false, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(stage, destination); err != nil {
		if hadDestination {
			return false, errors.Join(err, os.Rename(backup, destination))
		}
		return false, err
	}
	if hadDestination {
		if err := os.RemoveAll(backup); err != nil {
			return false, err
		}
	}
	return true, nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		return errors.Join(copyErr, closeErr)
	})
}

func treesEqual(a, b string) (bool, error) {
	if _, err := os.Stat(b); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	aFiles, err := treeFiles(a)
	if err != nil {
		return false, err
	}
	bFiles, err := treeFiles(b)
	if err != nil {
		return false, err
	}
	if len(aFiles) != len(bFiles) {
		return false, nil
	}
	for index := range aFiles {
		if aFiles[index].path != bFiles[index].path || aFiles[index].mode != bFiles[index].mode || !bytes.Equal(aFiles[index].body, bFiles[index].body) {
			return false, nil
		}
	}
	return true, nil
}

type treeFile struct {
	path string
	mode fs.FileMode
	body []byte
}

func treeFiles(root string) ([]treeFile, error) {
	var files []treeFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, treeFile{path: rel, mode: info.Mode().Perm(), body: body})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, err
}
