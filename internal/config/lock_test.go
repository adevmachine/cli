package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Every writer reads config.yml, changes it and writes it back. Run at once
// without the lock, the last write wins and the others are lost.
func TestConcurrentWritersLoseNothing(t *testing.T) {
	dir := writeConfig(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\nworkspaces:\n  - name: alice\n")
	const n = 40

	var wg sync.WaitGroup
	errs := make(chan error, 3*n)
	for i := range n {
		wg.Add(3)
		go func() {
			defer wg.Done()
			errs <- AddMachine(dir, Machine{Name: fmt.Sprintf("m%d", i),
				Hosts: []Host{{Address: fmt.Sprintf("203.0.113.%d", 20+i)}}, User: "root", Port: 22})
		}()
		go func() {
			defer wg.Done()
			errs <- AddWorkspace(dir, Workspace{Name: fmt.Sprintf("w%d", i), Machine: "main"})
		}()
		go func() {
			defer wg.Done()
			errs <- AddRoute(dir, "alice", Route{Host: fmt.Sprintf("s%d.example.com", i), Port: 8000 + i})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := c.Workspace("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Machines) != n+1 || len(c.Workspaces) != n+1 || len(alice.Routes) != n {
		t.Fatalf("lost writes: %d machines, %d workspaces, %d routes", len(c.Machines), len(c.Workspaces), len(alice.Routes))
	}
}

func TestLockIsHeldUntilReleased(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		second, err := Lock(dir)
		if err != nil {
			t.Error(err)
		} else {
			second()
		}
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("a second writer took the lock while the first held it")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	<-acquired
}

func TestAWriteThroughASymlinkKeepsTheLink(t *testing.T) {
	real := writeConfig(t, "machines:\n  - name: main\n    hosts: [203.0.113.10]\n")
	dir := t.TempDir()
	link := filepath.Join(dir, FileName)
	if err := os.Symlink(filepath.Join(real, FileName), link); err != nil {
		t.Fatal(err)
	}
	if err := SetSSHAliases(dir, true); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config.yml is no longer a link: %v", err)
	}
	c, err := Load(real)
	if err != nil || !c.SSHAliases {
		t.Fatalf("the linked file was not written: %v %#v", err, c)
	}
}
