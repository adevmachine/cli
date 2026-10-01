package commands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/doctor"
	"github.com/mydevmachine/devmachine/internal/remote"
)

func farUnreachable(t *testing.T) {
	t.Helper()
	client := &recordingRemote{out: "ID=ubuntu\n"}
	t.Cleanup(swap(&dial, func(_ context.Context, m config.Machine, _ string) (remote.Client, string, error) {
		if m.Name == "far" {
			return nil, "", errors.New("no address answered")
		}
		return client, "203.0.113.10", nil
	}))
}

func TestDoctorWithSeveralMachinesChecksEveryOne(t *testing.T) {
	dir := twoMachines(t)
	answering(t, "ID=ubuntu")

	out, err := execute(t, "--config", dir, "doctor")
	if err != nil {
		t.Fatalf("every machine is healthy, yet doctor failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "say which one") {
		t.Fatalf("doctor asked for --machine instead of checking both:\n%s", out)
	}
	main := strings.Index(out, "machine main:")
	far := strings.Index(out, "machine far:")
	if main < 0 || far < 0 || far < main {
		t.Fatalf("want one block per machine, main then far:\n%s", out)
	}
	if !strings.Contains(out[main:far], `machine "main"`) || !strings.Contains(out[far:], `machine "far"`) {
		t.Fatalf("a block does not hold its own machine's checks:\n%s", out)
	}
	if strings.Count(out, "  cli ") != 1 {
		t.Fatalf("the checks about this computer must appear once:\n%s", out)
	}
}

func TestDoctorWithSeveralMachinesFailsWhenOneFails(t *testing.T) {
	dir := twoMachines(t)
	farUnreachable(t)

	out, err := execute(t, "--config", dir, "doctor")
	if err == nil {
		t.Fatalf("an unreachable machine must fail doctor:\n%s", out)
	}
	if !strings.Contains(err.Error(), "far") || strings.Contains(err.Error(), "main") {
		t.Fatalf("the error must name only the failing machine: %v", err)
	}
	far := strings.Index(out, "machine far:")
	if far < 0 || !strings.Contains(lineWith(t, out[far:], "connection"), "fail") {
		t.Fatalf("far's connection failure is not in its block:\n%s", out)
	}
	main := strings.Index(out, "machine main:")
	if !strings.HasPrefix(strings.TrimSpace(lineWith(t, out[main:far], "connection")), "pass") {
		t.Fatalf("main was not checked on its own:\n%s", out)
	}
}

func TestDoctorWithAMachineNamedChecksOnlyThatOne(t *testing.T) {
	dir := twoMachines(t)
	farUnreachable(t)

	out, err := execute(t, "--config", dir, "--machine", "main", "doctor")
	if err != nil {
		t.Fatalf("far was not asked for: %v\n%s", err, out)
	}
	if strings.Contains(out, "machine main:") || strings.Contains(out, "far") {
		t.Fatalf("a single machine keeps the plain output:\n%s", out)
	}
	if !strings.HasPrefix(out, "pass  configuration") {
		t.Fatalf("a single machine keeps the plain output:\n%s", out)
	}
}

func TestDoctorJSONWithSeveralMachinesHasOneEntryPerMachine(t *testing.T) {
	dir := twoMachines(t)
	farUnreachable(t)

	out, err := execute(t, "--config", dir, "--format", "json", "doctor")
	if err == nil {
		t.Fatalf("an unreachable machine must fail doctor:\n%s", out)
	}
	var got struct {
		Machines []struct {
			Machine string         `json:"machine"`
			Checks  []doctor.Check `json:"checks"`
			OK      bool           `json:"ok"`
		} `json:"machines"`
		Checks []doctor.Check `json:"checks"`
		OK     *bool          `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got.Machines) != 2 || got.Machines[0].Machine != "main" || got.Machines[1].Machine != "far" {
		t.Fatalf("machines = %+v", got.Machines)
	}
	if !got.Machines[0].OK || got.Machines[1].OK {
		t.Fatalf("main should be ok and far not: %+v", got.Machines)
	}
	if got.OK == nil || *got.OK {
		t.Fatalf("the top-level ok must be false when any machine failed:\n%s", out)
	}
	names := map[string]bool{}
	for _, c := range got.Checks {
		names[c.Name] = true
	}
	if !names["cli"] || !names["packages pin"] || names["configuration"] {
		t.Fatalf("top-level checks are about this computer only: %+v", got.Checks)
	}
}

func TestDoctorJSONWithOneMachineKeepsItsShape(t *testing.T) {
	dir := configWithTrustedKey(t, "")
	answering(t, "ID=ubuntu")

	out, err := execute(t, "--config", dir, "--format", "json", "doctor")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if _, ok := got["machines"]; ok || len(got) != 2 || got["checks"] == nil || got["ok"] == nil {
		t.Fatalf("the single-machine shape changed: %s", out)
	}
}
