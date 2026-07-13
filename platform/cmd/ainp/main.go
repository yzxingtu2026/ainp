package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
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

	// 加载安装状态
	st, err := state.Load(cfg.StateFile)
	if err != nil {
		logging.Error(ctx, "load state failed", err)
		os.Exit(1)
	}

	if !st.Installed {
		runSetupMode(ctx, cfg, st)
		return
	}

	runNormalMode(ctx, cfg, cfgPath)
}

// runSetupMode 未安装：仅注册 setup 系列 API，不连接数据库。
func runSetupMode(ctx context.Context, cfg *config.Config, st *state.State) {
	logging.Info(ctx, "starting in setup mode", "addr", cfg.Server.Addr)

	// 加载翻译文件（setup 页面也需要 i18n）
	loadI18n(cfg)

	// 无 DB 时使用 setup-only auditor（nil 即可，setup.initialize 内部会处理）
	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		logging.Error(ctx, "init token service failed", err)
		os.Exit(1)
	}

	application := app.New(cfg, nil, nil, tokens)
	application.Installed = false

	setupSvc := system.NewSetupService(cfg.Database.DSN, cfg.StateFile, st)
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		logging.Error(ctx, "register setup failed", err)
		os.Exit(1)
	}

	logging.Info(ctx, "ainp setup wizard is ready",
		"actions", len(application.Actions.List()),
	)
	if err := application.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}

// runNormalMode 已安装：正常启动，连接数据库、注册所有模块。
func runNormalMode(ctx context.Context, cfg *config.Config, cfgPath *string) {
	logging.Info(ctx, "starting ainp", "addr", cfg.Server.Addr)

	// 校验已安装模式的必填项
	if err := cfg.Validate(true); err != nil {
		fmt.Fprintf(os.Stderr, "config validate: %v\n", err)
		os.Exit(1)
	}

	loadI18n(cfg)

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
	if err := system.AutoMigrate(db); err != nil {
		logging.Error(ctx, "system migrate failed", err)
		os.Exit(1)
	}
	if err := system.SeedDemo(ctx, db); err != nil {
		logging.Error(ctx, "system seed failed", err)
		os.Exit(1)
	}

	tokens, err := auth.NewTokenService(cfg.Auth.JWT.Secret, cfg.Auth.JWT.Issuer, cfg.Auth.JWT.Duration())
	if err != nil {
		logging.Error(ctx, "init token service failed", err)
		os.Exit(1)
	}

	application := app.New(cfg, db, auditor, tokens)

	// 注册 setup actions（已安装模式下 setup_initialize 会返回已安装错误）
	st, _ := state.Load(cfg.StateFile)
	if st == nil {
		st = &state.State{Installed: true}
	}
	setupSvc := system.NewSetupService(cfg.Database.DSN, cfg.StateFile, st)
	if err := system.RegisterSetupActions(application, setupSvc); err != nil {
		logging.Error(ctx, "register setup failed", err)
		os.Exit(1)
	}

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
		"dev_token", cfg.Auth.DevTokenEnabled,
	)
	if err := application.Run(); err != nil {
		logging.Error(ctx, "server stopped", err)
		os.Exit(1)
	}
}

func loadI18n(cfg *config.Config) {
	dir := cfg.I18n.Dir
	if dir == "" {
		dir = "i18n"
	}
	if err := i18n.LoadDir(dir); err != nil {
		logging.Error(context.Background(), "i18n load failed", err)
	}
	if cfg.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(cfg.I18n.DefaultLanguage)
	}
}
