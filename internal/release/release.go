// Package release answers "what is the newest published release" for the CLI
// and for the packages, and compares versions.
package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LookupTimeout bounds one question to GitHub. A check that hangs is worse
// than one that says "I cannot tell".
const LookupTimeout = 10 * time.Second

var client = &http.Client{Timeout: LookupTimeout}

var numbers = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)

// Compare orders two versions, numerically part by part: "v0.7.18" and
// "0.7.18" are the same, "v17" is newer than "v16", and a missing part counts
// as zero. Anything after the numbers (a pre-release suffix) is ignored.
//
// ok is false when either side has no version number to read, such as a
// development build that calls itself "dev".
func Compare(a, b string) (int, bool) {
	left, okA := parts(a)
	right, okB := parts(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range max(len(left), len(right)) {
		var x, y int
		if i < len(left) {
			x = left[i]
		}
		if i < len(right) {
			y = right[i]
		}
		if x != y {
			return x - y, true
		}
	}
	return 0, true
}

// Newer reports whether latest is a newer version than current. A version
// that cannot be read is never older than anything.
func Newer(latest, current string) bool {
	n, ok := Compare(latest, current)
	return ok && n > 0
}

func parts(v string) ([]int, bool) {
	match := numbers.FindStringSubmatch(strings.TrimSpace(v))
	if match == nil {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(match[1], ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Latest asks a GitHub "latest release" API URL for its tag, without
// authenticating.
func Latest(ctx context.Context, apiURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, LookupTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("asking for the latest release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// GitHub's API refuses a request with no User-Agent.
	req.Header.Set("User-Agent", "devmachine-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("asking for the latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asking for the latest release: GitHub answered %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("reading the latest release: %w", err)
	}
	if payload.TagName == "" {
		return "", errors.New("the latest release has no tag name")
	}
	return payload.TagName, nil
}
