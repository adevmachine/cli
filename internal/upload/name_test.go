package upload

import (
	"testing"
	"time"
)

var at = time.Date(2026, 9, 30, 14, 30, 12, 0, time.Local)

func TestNamePutsTheTimestampBeforeTheExtension(t *testing.T) {
	cases := map[string]string{
		"report.pdf":            "report-20260930-143012.pdf",
		"photo.final.png":       "photo.final-20260930-143012.png",
		"Makefile":              "Makefile-20260930-143012",
		".env":                  ".env-20260930-143012",
		"archive.tar.gz":        "archive.tar-20260930-143012.gz",
		"notes.":                "notes.-20260930-143012",
		"my report ção.txt":     "my report ção-20260930-143012.txt",
		"a/b.txt":               "a_b-20260930-143012.txt",
		"line\nbreak.txt":       "line_break-20260930-143012.txt",
		"carriage\rreturn.txt":  "carriage_return-20260930-143012.txt",
		"nul\x00byte.txt":       "nul_byte-20260930-143012.txt",
		"'; rm -rf ~; $(id).sh": "'; rm -rf ~; $(id)-20260930-143012.sh",
	}
	for in, want := range cases {
		stem, ext := Name(in, at)
		if got := stem + ext; got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNameSplitsTheStemFromTheExtension(t *testing.T) {
	stem, ext := Name("report.pdf", at)
	if stem != "report-20260930-143012" || ext != ".pdf" {
		t.Fatalf("got %q %q", stem, ext)
	}
}

func TestDirDefaultsToTheUploadsCache(t *testing.T) {
	got, err := Dir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != ".cache/devmachine/uploads" {
		t.Fatalf("got %q", got)
	}
}

func TestDirAcceptsPathsInsideTheHome(t *testing.T) {
	cases := map[string]string{
		"notes":            "notes",
		"notes/":           "notes",
		"./notes/./today":  "notes/today",
		"a/../notes":       "notes",
		".":                ".",
		"~":                ".",
		"~/":               ".",
		"~/notes":          "notes",
		"/home/acme/notes": "/home/acme/notes",
		"/home/acme/x/../": "/home/acme",
		"with space/ção":   "with space/ção",
	}
	for in, want := range cases {
		got, err := Dir(in)
		if err != nil {
			t.Errorf("Dir(%q) returned %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Dir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDirRefusesPathsThatClimbOutOfTheHome(t *testing.T) {
	for _, in := range []string{"..", "../bob", "notes/../../bob", "~/../bob", "~bob/notes", "a\nb", "a\x00b"} {
		if _, err := Dir(in); err == nil {
			t.Errorf("Dir(%q) was accepted", in)
		}
	}
}

func TestModeAcceptsOctalPermissions(t *testing.T) {
	for _, in := range []string{"600", "0600", "644", "0755"} {
		if _, err := Mode(in); err != nil {
			t.Errorf("Mode(%q) returned %v", in, err)
		}
	}
	for _, in := range []string{"", "rw", "999", "60", "06000", "0o600", "600; id"} {
		if _, err := Mode(in); err == nil {
			t.Errorf("Mode(%q) was accepted", in)
		}
	}
}
