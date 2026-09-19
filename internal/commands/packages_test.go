package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/adevmachine/cli/internal/packages"
)

func writeBrokenPackage(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "git")
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yml"), []byte("format: 1\nname: git\nscope: user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "---\n- name: Install git\n  apt:\n    name: git\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks", "main.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPackagesValidatePrintsEveryProblemAtOnce(t *testing.T) {
	dir := writeBrokenPackage(t)

	out, err := execute(t, "packages", "validate", dir)
	if err == nil {
		t.Fatal("a broken package passed")
	}
	for _, want := range []string{"scope", "summary", "apt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the output leaves out %q: %s", want, out)
		}
	}
}

func TestPackagesValidateAsJSONCarriesEachProblemWithItsPlace(t *testing.T) {
	dir := writeBrokenPackage(t)

	out, _ := execute(t, "--format", "json", "packages", "validate", dir)
	var got struct {
		OK       bool `json:"ok"`
		Problems []struct {
			File string `json:"file"`
			Line int    `json:"line"`
			What string `json:"what"`
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, out)
	}
	if got.OK || len(got.Problems) == 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestPackagesValidateAcceptsAGoodPackage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sharing")
	if err := packages.WriteSkeleton(dir, "sharing", packages.ScopeWorkspace); err != nil {
		t.Fatal(err)
	}

	if _, err := execute(t, "packages", "validate", dir); err != nil {
		t.Fatalf("the skeleton did not pass: %v", err)
	}
}

func TestPackagesNewWritesSomethingValidateAccepts(t *testing.T) {
	into := t.TempDir()

	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "packages", "validate", filepath.Join(into, "sharing")); err != nil {
		t.Fatalf("what `packages new` wrote does not validate: %v", err)
	}
}

func TestPackagesNewRefusesToOverwrite(t *testing.T) {
	into := t.TempDir()

	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "packages", "new", "sharing", "--scope", "workspace", "--into", into); err == nil {
		t.Fatal("it overwrote a package that was already there")
	}
}

func TestPackagesSchemaJSONNamesEveryField(t *testing.T) {
	out, err := execute(t, "packages", "schema", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Formats []int `json:"formats"`
		Fields  []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Summary  string `json:"summary"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	for _, want := range []string{"format", "name", "scope", "summary", "needs", "provides", "extends", "credentials"} {
		found := false
		for _, f := range got.Fields {
			if f.Name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("the schema leaves out %q", want)
		}
	}
	// Whoever writes a recipe has to know which format number to put at the
	// top, and asking the binary is the only answer that cannot drift.
	if !slices.Equal(got.Formats, packages.ReadableFormats) {
		t.Fatalf("the schema does not publish the formats it reads: %#v", got.Formats)
	}
}

func TestPackagesSchemaAsATablePrintsTheFormatAndTheFields(t *testing.T) {
	out, err := execute(t, "packages", "schema")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"format", "scope", "required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table leaves out %q: %s", want, out)
		}
	}
}
