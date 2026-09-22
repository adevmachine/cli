package expose

import (
	"strings"
	"testing"
)

func TestRenderProxiesToLocalhost(t *testing.T) {
	got := Render(Site{Host: "app.example.com", Port: 8080})

	for _, want := range []string{"app.example.com {", "reverse_proxy 127.0.0.1:8080", "}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the block leaves out %q:\n%s", want, got)
		}
	}
}

func TestRenderSaysItIsGenerated(t *testing.T) {
	// Somebody will find this file on the machine and wonder whether to edit
	// it. Tell them there, not in a document they will not be reading.
	got := Render(Site{Host: "app.example.com", Port: 8080})
	if !strings.Contains(got, "devmachine expose") {
		t.Fatalf("the block does not say what wrote it:\n%s", got)
	}
}

func TestRenderNamesTheWorkspaceWhenThereIsOne(t *testing.T) {
	got := Render(Site{Host: "app.example.com", Port: 8080, Workspace: "alice"})
	if !strings.Contains(got, "alice") {
		t.Fatalf("the block does not name the workspace:\n%s", got)
	}
}

func TestFileNameIsTheHost(t *testing.T) {
	if got := FileName(Site{Host: "app.example.com"}); got != "app.example.com.caddy" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderRefusesAHostThatWouldEscapeTheFileName(t *testing.T) {
	// The host arrives from a command line and becomes a file name on a
	// machine. Nothing about that is safe by accident.
	for _, host := range []string{"../etc/passwd", "a/b.example.com", ""} {
		if FileName(Site{Host: host}) != "" {
			t.Fatalf("%q was accepted as a file name", host)
		}
	}
}
