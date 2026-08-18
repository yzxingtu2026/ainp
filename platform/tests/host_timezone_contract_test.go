package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const embeddedTimezoneHelperEnv = "AINP_TEST_EMBEDDED_TIMEZONE_HELPER"

func TestPublicHostEmbedsIANATimezoneData(t *testing.T) {
	platformRoot := filepath.Clean("..")
	goCache := filepath.Join(t.TempDir(), "go-build")
	for _, target := range []string{"./host", "./cmd/ainp", "./cmd/ainp-worker", "./cmd/ainp-migrate"} {
		command := exec.Command("go", "list", "-deps", target)
		command.Dir = platformRoot
		command.Env = append(os.Environ(), "GOCACHE="+goCache)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("list dependencies for %s: %v\n%s", target, err, output)
		}
		if !dependencyListContains(string(output), "time/tzdata") {
			t.Fatalf("%s must include time/tzdata through the public Host", target)
		}
	}
}

func TestEmbeddedIANATimezoneBehavior(t *testing.T) {
	if os.Getenv(embeddedTimezoneHelperEnv) == "1" {
		if err := assertEmbeddedIANATimezones(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	missingRoot := filepath.Join(t.TempDir(), "missing")
	command := exec.Command(os.Args[0], "-test.run=^TestEmbeddedIANATimezoneBehavior$")
	command.Env = append(os.Environ(),
		embeddedTimezoneHelperEnv+"=1",
		"ZONEINFO="+filepath.Join(missingRoot, "zoneinfo.zip"),
		"GOROOT="+missingRoot,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("standalone timezone probe failed without external zoneinfo: %v\n%s", err, output)
	}
}

func assertEmbeddedIANATimezones() error {
	for _, test := range []struct {
		name       string
		at         time.Time
		wantOffset int
	}{
		{name: "Asia/Shanghai", at: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), wantOffset: 8 * 60 * 60},
		{name: "America/New_York", at: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), wantOffset: -4 * 60 * 60},
	} {
		location, err := time.LoadLocation(test.name)
		if err != nil {
			return fmt.Errorf("load embedded IANA timezone %s: %w", test.name, err)
		}
		_, offset := test.at.In(location).Zone()
		if offset != test.wantOffset {
			return fmt.Errorf("timezone %s offset = %d, want %d", test.name, offset, test.wantOffset)
		}
	}
	return nil
}

func dependencyListContains(output, dependency string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == dependency {
			return true
		}
	}
	return false
}
