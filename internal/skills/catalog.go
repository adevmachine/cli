// Package skills discovers and installs Agent Skills contributed by packages.
package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var skillName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Skill is one complete skill directory.
type Skill struct {
	Name string
	Path string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Discover validates and returns every direct child skill in root.
func Discover(root string) ([]Skill, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("reading skill directory %s: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("skill path %s must be a real directory", root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading skill directory %s: %w", root, err)
	}

	var found []Skill
	var problems []error
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			problems = append(problems, fmt.Errorf("%s is a symlink; skill paths must stay inside %s", path, root))
			continue
		}
		if !entry.IsDir() {
			problems = append(problems, fmt.Errorf("%s is not a skill directory", path))
			continue
		}
		if !skillName.MatchString(entry.Name()) {
			problems = append(problems, fmt.Errorf("skill directory %q: use lower case letters, digits and dashes", entry.Name()))
			continue
		}
		if err := rejectSymlinks(path); err != nil {
			problems = append(problems, err)
			continue
		}
		meta, err := readFrontmatter(filepath.Join(path, "SKILL.md"))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if meta.Name != entry.Name() {
			problems = append(problems, fmt.Errorf("%s: name %q must match directory %q", filepath.Join(path, "SKILL.md"), meta.Name, entry.Name()))
			continue
		}
		if strings.TrimSpace(meta.Description) == "" {
			problems = append(problems, fmt.Errorf("%s: description must not be empty", filepath.Join(path, "SKILL.md")))
			continue
		}
		found = append(found, Skill{Name: entry.Name(), Path: path})
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, errors.Join(problems...)
}

func rejectSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; skill paths must stay inside their package", path)
		}
		return nil
	})
}

func readFrontmatter(path string) (frontmatter, error) {
	var meta frontmatter
	body, err := os.ReadFile(path)
	if err != nil {
		return meta, fmt.Errorf("reading %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return meta, fmt.Errorf("%s: SKILL.md must start with YAML frontmatter", path)
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return meta, fmt.Errorf("%s: SKILL.md frontmatter has no closing ---", path)
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return meta, fmt.Errorf("parsing %s frontmatter: %w", path, err)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return meta, fmt.Errorf("%s: name must not be empty", path)
	}
	return meta, nil
}
