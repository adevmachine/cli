package stats

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func fixtures(t *testing.T) (free, df, loadavg, nproc string) {
	t.Helper()
	return fixture(t, "free.txt"), fixture(t, "df.txt"),
		fixture(t, "loadavg.txt"), fixture(t, "nproc.txt")
}

func TestParseReadsMemory(t *testing.T) {
	got, err := Parse(fixtures(t))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	if got.MemTotalBytes != 4094312448 {
		t.Fatalf("total memory = %d", got.MemTotalBytes)
	}
	if got.MemUsedBytes != 441991168 {
		t.Fatalf("used memory = %d", got.MemUsedBytes)
	}
	if got.MemAvailableBytes != 3652321280 {
		t.Fatalf("available memory = %d", got.MemAvailableBytes)
	}
}

func TestParseReadsSwap(t *testing.T) {
	got, _ := Parse(fixtures(t))
	if got.SwapTotalBytes != 0 || got.SwapUsedBytes != 0 {
		t.Fatalf("swap = %d/%d, want 0/0 on this fixture", got.SwapUsedBytes, got.SwapTotalBytes)
	}
}

func TestParseReadsDisk(t *testing.T) {
	got, _ := Parse(fixtures(t))
	if got.DiskTotalBytes != 19682557952 {
		t.Fatalf("total disk = %d", got.DiskTotalBytes)
	}
	if got.DiskUsedBytes != 2774286336 {
		t.Fatalf("used disk = %d", got.DiskUsedBytes)
	}
}

func TestParseReadsLoadAndCPUs(t *testing.T) {
	got, _ := Parse(fixtures(t))
	if got.Load1 != 0 || got.Load5 != 0 || got.Load15 != 0 {
		t.Fatalf("load = %v/%v/%v", got.Load1, got.Load5, got.Load15)
	}
	if got.CPUs != 2 {
		t.Fatalf("cpus = %d, want 2", got.CPUs)
	}
}

func TestUsedIsNeverAboveTotal(t *testing.T) {
	got, _ := Parse(fixtures(t))
	if got.MemUsedBytes > got.MemTotalBytes {
		t.Fatalf("used memory %d is above total %d", got.MemUsedBytes, got.MemTotalBytes)
	}
	if got.DiskUsedBytes > got.DiskTotalBytes {
		t.Fatalf("used disk %d is above total %d", got.DiskUsedBytes, got.DiskTotalBytes)
	}
}

func TestParseRejectsOutputItDoesNotUnderstand(t *testing.T) {
	_, df, loadavg, nproc := fixtures(t)

	_, err := Parse("this is not free output", df, loadavg, nproc)
	if err == nil {
		t.Fatal("expected an error for output Parse cannot read")
	}
	if !strings.Contains(err.Error(), "free") {
		t.Fatalf("the error does not say which output was wrong: %v", err)
	}
}

func TestParseRejectsADiskLineItCannotRead(t *testing.T) {
	free, _, loadavg, nproc := fixtures(t)

	if _, err := Parse(free, "Filesystem 1B-blocks\n", loadavg, nproc); err == nil {
		t.Fatal("expected an error for a df output with no data row")
	}
}

func TestParseSurvivesAMissingNproc(t *testing.T) {
	free, df, loadavg, _ := fixtures(t)

	got, err := Parse(free, df, loadavg, "")
	if err != nil {
		t.Fatalf("a missing nproc should not fail the whole snapshot: %v", err)
	}
	if got.CPUs != 0 {
		t.Fatalf("cpus = %d, want 0 when nproc said nothing", got.CPUs)
	}
}

type fakeClient struct {
	out map[string]string
	err error
}

func (f fakeClient) Run(_ context.Context, command string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.out[command], nil
}

func (f fakeClient) Stream(ctx context.Context, command string, stdout, _ io.Writer) error {
	out, err := f.Run(ctx, command)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, out)
	return err
}

func (f fakeClient) RunInput(ctx context.Context, command string, _ io.Reader) (string, error) {
	return f.Run(ctx, command)
}

func (f fakeClient) Upload(context.Context, string, io.Reader) error { return nil }

func (f fakeClient) Close() error { return nil }

func TestCollectAsksForEveryPieceItNeeds(t *testing.T) {
	free, df, loadavg, nproc := fixtures(t)
	client := fakeClient{out: map[string]string{
		freeCommand:    free,
		dfCommand:      df,
		loadavgCommand: loadavg,
		nprocCommand:   nproc,
	}}

	got, err := Collect(context.Background(), client)
	if err != nil {
		t.Fatalf("Collect returned %v", err)
	}
	if got.MemTotalBytes == 0 || got.DiskTotalBytes == 0 || got.CPUs == 0 {
		t.Fatalf("Collect did not fill the snapshot: %#v", got)
	}
}

func TestCollectReportsAConnectionThatFailed(t *testing.T) {
	client := fakeClient{err: errors.New("connection lost")}

	if _, err := Collect(context.Background(), client); err == nil {
		t.Fatal("expected an error when the commands cannot run")
	}
}
