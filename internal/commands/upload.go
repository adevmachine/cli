package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mydevmachine/devmachine/internal/remote"
	"github.com/mydevmachine/devmachine/internal/upload"
	"github.com/spf13/cobra"
)

// uploadNow is the clock the file names are stamped with.
var uploadNow = time.Now

type uploadResult struct {
	Local  string `json:"local"`
	Remote string `json:"remote,omitempty"`
	Bytes  int64  `json:"bytes"`
	Error  string `json:"error,omitempty"`
	err    error
}

func newUploadCmd(opts *options) *cobra.Command {
	var workspace, dir, mode string

	c := &cobra.Command{
		Use:   "upload <file>...",
		Short: "Send local files into a workspace's or a machine's home",
		Long: "Sends each file into the home of a workspace's account with " +
			"--workspace, or of the machine's admin with --machine. With " +
			"neither, it acts on the only machine configured.\n\n" +
			"Files land in ~/" + upload.DefaultDir + " unless --dir names another " +
			"folder inside the same home. Each keeps its name with the local time " +
			"before the extension (report.pdf becomes report-20260930-143012.pdf), " +
			"and a name already taken gets -2, -3 and so on: nothing is ever " +
			"overwritten.\n\n" +
			"Prints the path each file landed at, one per line. With several " +
			"files it tries all of them and exits non-zero if any failed.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(cmd, opts, workspace, dir, mode, args)
		},
	}
	c.Flags().StringVar(&workspace, "workspace", "", "send into this workspace's home instead of the machine admin's")
	c.Flags().StringVar(&dir, "dir", "", "folder inside the home to send into (default ~/"+upload.DefaultDir+")")
	c.Flags().StringVar(&mode, "mode", "0600", "permissions of the files on the machine")
	return c
}

func runUpload(cmd *cobra.Command, opts *options, workspace, dir, mode string, files []string) error {
	dest, err := upload.Dir(dir)
	if err != nil {
		return err
	}
	if _, err := upload.Mode(mode); err != nil {
		return err
	}
	tgt, err := transferTarget(opts, workspace, "upload")
	if err != nil {
		return err
	}

	results := make([]uploadResult, len(files))
	pending := 0
	for i, f := range files {
		results[i] = uploadResult{Local: f}
		size, err := checkUploadable(f)
		if err != nil {
			results[i].err = err
			continue
		}
		results[i].Bytes = size
		pending++
	}

	if pending > 0 {
		client, _, err := dialMux(cmd.Context(), tgt.machine, tgt.user)
		if err != nil {
			return err
		}
		defer client.Close()

		at := uploadNow()
		for i := range results {
			r := &results[i]
			if r.err != nil {
				continue
			}
			r.Remote, r.Bytes, r.err = sendFile(cmd, client, dest, mode, r.Local, at)
			record(opts, tgt, "upload "+r.Local, r.err == nil)
			if errors.Is(r.err, remote.ErrHostKeyRejected) {
				return explainHostKey(cmd.Context(), tgt.machine, r.err)
			}
		}
	}

	return reportUploads(cmd, opts, results)
}

// transferTarget is workspaceTarget, with one more refusal: a workspace named
// together with a --machine it does not live on is a mistake to report, not
// a choice to make for the person.
func transferTarget(opts *options, workspace, command string) (target, error) {
	tgt, err := workspaceTarget(opts, workspace)
	if err != nil {
		return target{}, err
	}
	if workspace != "" && opts.machine != "" && opts.machine != tgt.machine.Name {
		return target{}, fmt.Errorf("workspace %s lives on machine %s, not %s: drop --machine or name the right one",
			workspace, tgt.machine.Name, opts.machine)
	}
	if err := requiresAddress(tgt.machine, command); err != nil {
		return target{}, err
	}
	return tgt, nil
}

// checkUploadable refuses what cannot be sent before any connection is made.
func checkUploadable(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%s is a folder: upload sends files; for a folder, send an archive of it "+
			"(tar czf %s.tgz -C %s %s)", path, filepath.Base(path), filepath.Dir(path), filepath.Base(path))
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("%s cannot be read: %w", path, err)
	}
	f.Close()
	return info.Size(), nil
}

func sendFile(cmd *cobra.Command, client remote.Client, dest, mode, path string, at time.Time) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("%s cannot be read: %w", path, err)
	}
	defer f.Close()

	stem, ext := upload.Name(filepath.Base(path), at)
	counted := &countingReader{r: f}
	landed, err := upload.Send(cmd.Context(), client, upload.Request{Dir: dest, Stem: stem, Ext: ext, Mode: mode}, counted)
	if err != nil {
		return "", counted.n, fmt.Errorf("%s: %w", path, err)
	}
	return landed, counted.n, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func reportUploads(cmd *cobra.Command, opts *options, results []uploadResult) error {
	var failed []error
	for i := range results {
		if results[i].err != nil {
			results[i].Error = results[i].err.Error()
			failed = append(failed, results[i].err)
		}
	}

	if opts.format == formatJSON {
		if err := writeJSON(cmd.OutOrStdout(), results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			if r.err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), r.Remote)
			}
		}
	}

	switch {
	case len(failed) == 0:
		return nil
	case len(results) == 1:
		return failed[0]
	}
	if opts.format != formatJSON {
		for _, err := range failed {
			fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		}
	}
	return fmt.Errorf("%d of %d files were not uploaded", len(failed), len(results))
}
