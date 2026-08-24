package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/host"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to yaml config")
	baselineThrough := flag.Int("through", 0, "baseline existing built-in schema through this version")
	planPath := flag.String("plan", "", "versioned adoption plan JSON path")
	confirmDigest := flag.String("confirm-digest", "", "required adoption plan digest for apply")
	flag.Parse()
	if flag.NArg() < 1 || (flag.Arg(0) != "status" && flag.Arg(0) != "up" && flag.Arg(0) != "baseline" && flag.Arg(0) != "adopt") {
		fmt.Fprintln(os.Stderr, "usage: ainp-migrate [-config path] status|up|baseline|adopt plan|dry-run|apply|verify")
		os.Exit(2)
	}
	if flag.Arg(0) == "adopt" && (flag.NArg() != 2 || *planPath == "") {
		fmt.Fprintln(os.Stderr, "usage: ainp-migrate -plan plan.json adopt plan|dry-run|apply|verify")
		os.Exit(2)
	}
	if flag.Arg(0) == "adopt" && flag.Arg(1) != "plan" && flag.Arg(1) != "dry-run" && flag.Arg(1) != "apply" && flag.Arg(1) != "verify" {
		fmt.Fprintln(os.Stderr, "usage: ainp-migrate -plan plan.json adopt plan|dry-run|apply|verify")
		os.Exit(2)
	}
	var plan migrations.AdoptionPlan
	if flag.Arg(0) == "adopt" {
		body, err := os.ReadFile(*planPath)
		if err != nil {
			fail(fmt.Errorf("read adoption plan: %w", err))
		}
		if err := json.Unmarshal(body, &plan); err != nil {
			fail(fmt.Errorf("decode adoption plan: %w", err))
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	for _, warning := range cfg.DeprecationWarnings() {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	st, err := state.Load(cfg.StateFile)
	if err != nil {
		fail(fmt.Errorf("load state: %w", err))
	}
	if st.Installed {
		st.ApplyConfig(cfg)
	}
	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		fail(err)
	}
	composition, err := host.New()
	if err != nil {
		fail(err)
	}
	runner := composition.Migrations()
	if flag.Arg(0) == "adopt" {
		if flag.Arg(1) == "plan" || flag.Arg(1) == "dry-run" || flag.Arg(1) == "verify" {
			result, err := plan.Inspect(db, runner)
			if err != nil {
				fail(err)
			}
			encoded, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(encoded))
			return
		}
		if flag.Arg(1) == "apply" {
			if *confirmDigest == "" {
				fail(fmt.Errorf("-confirm-digest is required for adoption apply"))
			}
			result, err := plan.Apply(db, runner, migrations.BridgeRegistry{}, *confirmDigest)
			if err != nil {
				fail(err)
			}
			encoded, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(encoded))
			return
		}
	}
	var status migrations.SetStatus
	if flag.Arg(0) == "up" {
		status, err = runner.Up(db)
	} else if flag.Arg(0) == "baseline" {
		if *baselineThrough > 0 {
			status, err = runner.BaselineThrough(db, *baselineThrough)
		} else {
			status, err = runner.Baseline(db)
		}
	} else {
		status, err = runner.Inspect(db)
	}
	if err != nil {
		fail(err)
	}
	if !status.Initialized {
		fmt.Printf("database is not initialized; setup will create the current schema (%d built-in, %d downstream migrations tracked)\n", len(status.Builtin.Pending), len(status.PendingDownstream))
		return
	}
	if flag.Arg(0) == "status" {
		fmt.Printf("applied=%d pending=%d downstream_applied=%d downstream_pending=%d\n", len(status.Builtin.Applied), len(status.Builtin.Pending), len(status.AppliedDownstream), len(status.PendingDownstream))
		for _, migration := range status.Builtin.Pending {
			fmt.Printf("pending %s\n", migration.Name)
		}
		for _, migration := range status.PendingDownstream {
			fmt.Printf("pending %s:%04d %s\n", migration.Owner, migration.Version, migration.Name)
		}
		return
	}
	fmt.Printf("migration %s complete: applied=%d pending=%d downstream_applied=%d downstream_pending=%d\n", flag.Arg(0), len(status.Builtin.Applied), len(status.Builtin.Pending), len(status.AppliedDownstream), len(status.PendingDownstream))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migration failed:", err)
	os.Exit(1)
}
