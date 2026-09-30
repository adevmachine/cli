package expose

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"strings"
)

// Outcome is what the machine answered to ApplyScript.
type Outcome string

// The answers ApplyScript gives, one per way it can end.
const (
	Applied      Outcome = "applied"
	Unchanged    Outcome = "unchanged"
	NoCaddy      Outcome = "no-caddy"
	Invalid      Outcome = "invalid"
	ReloadFailed Outcome = "reload-failed"
)

const verdictPrefix = "devmachine-apply: "

// Caddyfile is the configuration the caddy package writes and its service
// loads, and so the one a new site file is validated against.
const Caddyfile = "/etc/caddy/Caddyfile"

// ApplyCommand runs ApplyScript, fed on stdin, as root: directly when the
// admin login already is root, through sudo when it is not.
const ApplyCommand = `if [ "$(id -u)" -eq 0 ]; then sh -s; else sudo -n sh -s; fi`

// ApplyScript makes one routes file what the change says, and reloads Caddy.
//
// The new file goes in place before `caddy validate` runs, because the
// Caddyfile imports sites.d/*.caddy and nothing else would validate the whole
// configuration with it. What it replaces is set aside under a name the glob
// does not match, and put back when Caddy refuses: a refused file never
// reaches a reload.
func ApplyScript(c FileChange, caddyfile string) string {
	dir := path.Dir(c.Path)
	fresh := path.Join(dir, "."+path.Base(c.Path)+".devmachine-new")
	targets := append([]string{c.Path}, c.Stale...)
	verdict := func(o Outcome) string { return "echo " + quote(verdictPrefix+string(o)) }

	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("set -u")
	line("exec 2>&1")
	line("if ! command -v caddy >/dev/null 2>&1 || [ ! -d %s ]; then %s; exit 0; fi", quote(dir), verdict(NoCaddy))

	same := make([]string, 0, len(targets))
	if c.Content != nil {
		line("umask 077")
		line("printf '%%s' %s | base64 -d > %s || { rm -f %s; exit 1; }",
			quote(base64.StdEncoding.EncodeToString(c.Content)), quote(fresh), quote(fresh))
		line("chown %s %s && chmod %s %s || { rm -f %s; exit 1; }",
			quote(c.Owner+":"+c.Group), quote(fresh), quote(c.Mode), quote(fresh), quote(fresh))
		same = append(same, fmt.Sprintf("cmp -s %s %s", quote(fresh), quote(c.Path)))
	} else {
		same = append(same, fmt.Sprintf("[ ! -e %s ]", quote(c.Path)))
	}
	for _, s := range c.Stale {
		same = append(same, fmt.Sprintf("[ ! -e %s ]", quote(s)))
	}
	line("if %s; then rm -f %s; %s; exit 0; fi", strings.Join(same, " && "), quote(fresh), verdict(Unchanged))

	var restore strings.Builder
	fmt.Fprintf(&restore, "rm -f %s %s", quote(fresh), quote(c.Path))
	for i, t := range targets {
		line("kept%d=0; if [ -e %s ]; then mv -f %s %s || exit 1; kept%d=1; fi",
			i, quote(t), quote(t), quote(t+".devmachine-old"), i)
		fmt.Fprintf(&restore, "; if [ \"$kept%d\" = 1 ]; then mv -f %s %s; fi", i, quote(t+".devmachine-old"), quote(t))
	}
	line("restore() { %s; }", restore.String())
	if c.Content != nil {
		line("mv -f %s %s || { restore; exit 1; }", quote(fresh), quote(c.Path))
	}

	config := quote(caddyfile)
	line("if ! out=$(caddy validate --config %s --adapter caddyfile 2>&1); then restore; printf '%%s\\n' \"$out\"; %s; exit 0; fi",
		config, verdict(Invalid))
	line("if ! out=$(systemctl reload caddy 2>&1) && ! out=$(caddy reload --config %s --adapter caddyfile 2>&1); then "+
		"restore; printf '%%s\\n' \"$out\"; %s; exit 0; fi", config, verdict(ReloadFailed))
	for _, t := range targets {
		line("rm -f %s", quote(t+".devmachine-old"))
	}
	line("%s", verdict(Applied))
	return b.String()
}

// ParseApply reads ApplyScript's answer: the verdict on its last line, and
// whatever Caddy said before it.
func ParseApply(out string) (Outcome, string, error) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, verdictPrefix) {
		return "", "", errors.New("the machine did not say whether the site was applied: " + strings.TrimSpace(out))
	}
	outcome := Outcome(strings.TrimPrefix(last, verdictPrefix))
	switch outcome {
	case Applied, Unchanged, NoCaddy, Invalid, ReloadFailed:
	default:
		return "", "", fmt.Errorf("the machine answered %q, which this version does not know", outcome)
	}
	return outcome, strings.TrimSpace(strings.Join(lines[:len(lines)-1], "\n")), nil
}

// Diff marks each line of two small texts as kept (" "), removed ("-") or
// added ("+"). Routes files are a few lines long, so the quadratic table is
// never large.
func Diff(before, after string) string {
	a := splitLines(before)
	b := splitLines(after)
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			out.WriteString(" " + a[i] + "\n")
			i++
			j++
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			out.WriteString("+" + b[j] + "\n")
			j++
		default:
			out.WriteString("-" + a[i] + "\n")
			i++
		}
	}
	return out.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
