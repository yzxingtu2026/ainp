package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to yaml config")
	flag.Parse()
	if flag.NArg() != 1 || (flag.Arg(0) != "status" && flag.Arg(0) != "up" && flag.Arg(0) != "baseline") {
		fmt.Fprintln(os.Stderr, "usage: ainp-migrate [-config path] status|up|baseline")
		os.Exit(2)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		fail(err)
	}
	var status migrations.Status
	if flag.Arg(0) == "up" {
		status, err = migrations.Up(db)
	} else if flag.Arg(0) == "baseline" {
		status, err = migrations.Baseline(db)
	} else {
		status, err = migrations.Inspect(db)
	}
	if err != nil {
		fail(err)
	}
	if !status.Initialized {
		fmt.Printf("database is not initialized; setup will create the current schema (%d migrations tracked)\n", len(status.Pending))
		return
	}
	if flag.Arg(0) == "status" {
		fmt.Printf("applied=%d pending=%d\n", len(status.Applied), len(status.Pending))
		for _, migration := range status.Pending {
			fmt.Printf("pending %s\n", migration.Name)
		}
		return
	}
	fmt.Printf("migration %s complete: applied=%d pending=%d\n", flag.Arg(0), len(status.Applied), len(status.Pending))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migration failed:", err)
	os.Exit(1)
}
