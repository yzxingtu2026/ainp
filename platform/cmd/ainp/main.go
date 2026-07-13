package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/modules/demo"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to yaml config")
	flag.Parse()

	ctx := context.Background()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logging.Init(cfg.Logging.Level, cfg.Logging.Format)
	logging.Info(ctx, "starting ainp", "addr", cfg.Server.Addr)

	db, err := database.Open(cfg.Database.DSN)
	if err != nil {
		logging.Error(ctx, "open database failed", err)
		os.Exit(1)
	}
	if err := database.Ping(db); err != nil {
		logging.Error(ctx, "ping database failed", err)
		os.Exit(1)
	}

	auditor := audit.NewGormRecorder(db)
	if err := auditor.AutoMigrate(); err != nil {
		logging.Error(ctx, "audit migrate failed", err)
		os.Exit(1)
	}

	application := app.New(cfg, db, auditor)
	if err := system.Register(application); err != nil {
		logging.Error(ctx, "register system failed", err)
		os.Exit(1)
	}
	if err := demo.Register(application); err != nil {
		logging.Error(ctx, "register demo failed", err)
		os.Exit(1)
	}

	logging.Info(ctx, "ainp is ready",
		"actions", len(application.Actions.List()),
		"menus", len(application.Menus.List()),
	)
	if err := application.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}
