package selfupdate

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxArchive bounds a download. The real archive is a few megabytes; this
// only stops a broken server from filling the disk.
const maxArchive = 256 << 20

var downloadClient = &http.Client{Timeout: 5 * time.Minute}

// ArchiveName is the release asset for one platform, as the release
// pipeline names it.
func ArchiveName(tag, goos, goarch string) string {
	return fmt.Sprintf("devmachine_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), goos, goarch)
}

// Downloader installs a release from its archive.
type Downloader struct {
	BaseURL string
	OS      string
	Arch    string
}

// Replace downloads the release tagged tag, checks it against the release's
// checksums.txt, and swaps it in for the binary at executable.
//
// The new file is written next to the old one and renamed over it, so the
// swap is atomic: a failure at any point leaves the old binary exactly as it
// was, and a process already running it keeps running.
func (d Downloader) Replace(ctx context.Context, tag, executable string) error {
	archive := ArchiveName(tag, d.OS, d.Arch)
	base := strings.TrimSuffix(d.BaseURL, "/") + "/" + tag + "/"

	sums, err := d.get(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, archive)
	if err != nil {
		return err
	}
	body, err := d.get(ctx, base+archive, maxArchive)
	if err != nil {
		return err
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s: the download is not what the release published, so nothing was replaced", archive)
	}
	binary, err := extractBinary(body)
	if err != nil {
		return fmt.Errorf("reading %s: %w", archive, err)
	}
	return swap(executable, binary)
}

func (d Downloader) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "devmachine-cli")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: the server answered %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("downloading %s: larger than %d bytes", url, limit)
	}
	return body, nil
}

func checksumFor(sums []byte, archive string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == archive {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not listed in the release's checksums.txt", archive)
}

func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("it holds no devmachine binary")
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "devmachine" {
			return io.ReadAll(io.LimitReader(tr, maxArchive))
		}
	}
}

func swap(executable string, binary []byte) error {
	target, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("finding the running binary %s: %w", executable, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("reading %s: %w", target, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".devmachine-update-*")
	if err != nil {
		return fmt.Errorf("writing next to %s: %w", target, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("writing the new binary: %w", err)
	}
	if err := tmp.Chmod(info.Mode().Perm() | 0o111); err != nil {
		tmp.Close()
		return fmt.Errorf("making the new binary executable: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the new binary: %w", err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return fmt.Errorf("replacing %s: %w", target, err)
	}
	return nil
}
