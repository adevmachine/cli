package provision

import (
	"slices"
	"testing"
)

func TestChangedTasksNamesEachTaskThatChangedOnce(t *testing.T) {
	output := `
PLAY [main] ********************************************************************

TASK [Gathering Facts] *********************************************************
ok: [main]

TASK [workspace : Create the account] ******************************************
changed: [main]

TASK [zsh : Install zsh] *******************************************************
ok: [main]

TASK [zsh : Link the dotfiles] *************************************************
changed: [main] => (item=.zshrc)
changed: [main] => (item=.zprofile)

RUNNING HANDLER [caddy : reload caddy] *****************************************
changed: [main]

PLAY RECAP *********************************************************************
main                       : ok=5    changed=3    unreachable=0    failed=0
`
	want := []string{"workspace : Create the account", "zsh : Link the dotfiles", "caddy : reload caddy"}
	if got := ChangedTasks(output); !slices.Equal(got, want) {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestChangedTasksWithNothingChanged(t *testing.T) {
	if got := ChangedTasks("TASK [x] ***\nok: [main]\n"); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}
