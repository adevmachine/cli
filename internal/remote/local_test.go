package remote

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/adevmachine/cli/internal/config"
)

func TestDialReturnsALocalClientForASelfMachine(t *testing.T) {
	client, address, err := Dial(context.Background(), config.Machine{Name: "mac", Self: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if address != SelfAddress {
		t.Fatalf("got address %q", address)
	}
	if _, ok := client.(*localClient); !ok {
		t.Fatalf("got %T, want a local client", client)
	}
}

func TestLocalClientRunsACommand(t *testing.T) {
	client := &localClient{}
	out, err := client.Run(context.Background(), "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi\n" {
		t.Fatalf("got %q", out)
	}
}

func TestLocalClientStreamsOutputAsItArrives(t *testing.T) {
	client := &localClient{}
	var out bytes.Buffer
	if err := client.Stream(context.Background(), "echo one", &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if out.String() != "one\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestLocalClientReportsANonZeroExitAsAnError(t *testing.T) {
	client := &localClient{}
	if _, err := client.Run(context.Background(), "exit 3"); err == nil {
		t.Fatal("a non-zero exit was reported as success")
	}
	if err := client.Stream(context.Background(), "exit 3", io.Discard, io.Discard); err == nil {
		t.Fatal("a non-zero exit was reported as success")
	}
}

func TestLocalClientRunInputFeedsStdin(t *testing.T) {
	client := &localClient{}
	out, err := client.RunInput(context.Background(), "cat", bytes.NewReader([]byte("hello\n")))
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\n" {
		t.Fatalf("got %q", out)
	}
}

func TestLocalClientUploadExtractsATarballIntoADirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	tarball := tarballWith(t, map[string]string{
		"ansible.cfg":         "[defaults]\n",
		"roles/base/task.yml": "---\n",
	})

	client := &localClient{}
	if err := client.Upload(context.Background(), dir, bytes.NewReader(tarball)); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(dir, "ansible.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[defaults]\n" {
		t.Fatalf("got %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "roles/base/task.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestLocalClientUploadRefusesAnEntryEscapingTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	tarball := tarballWith(t, map[string]string{"../escaped.txt": "nope\n"})

	client := &localClient{}
	if err := client.Upload(context.Background(), dir, bytes.NewReader(tarball)); err == nil {
		t.Fatal("an entry outside the directory was extracted without complaint")
	}
}
