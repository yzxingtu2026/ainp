package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SetupService 安装向导核心服务。
type SetupService struct {
	cfg             *config.Config
	statePath       string       // state.yaml 路径
	st              *state.State // 当前状态
	onInstalled     func()       // 安装完成后的回调（用于更新内存状态）
	initialDataHook func(context.Context, *gorm.DB, string, string) error
}

func (s *SetupService) SetInitialDataHook(hook func(context.Context, *gorm.DB, string, string) error) {
	s.initialDataHook = hook
}

func NewSetupService(cfg *config.Config, statePath string, st *state.State, onInstalled func()) *SetupService {
	return &SetupService{cfg: cfg, statePath: statePath, st: st, onInstalled: onInstalled}
}

type setupStatusResp struct {
	Installed    bool   `json:"installed"`
	DSNSource    string `json:"dsn_source"`
	DSNConnected bool   `json:"dsn_connected"`
}

func (s *SetupService) Status(_ context.Context, _ json.RawMessage) (any, error) {
	src := "none"
	if os.Getenv("DATABASE_DSN") != "" {
		src = "env"
	} else if s.cfg.Database.DSN != "" {
		src = "config"
	}
	connected := false
	if s.cfg.Database.DSN != "" || s.st.DSN != "" {
		dsn := s.cfg.Database.DSN
		if dsn == "" {
			dsn = s.st.DSN
		}
		db, err := database.Open(dsn)
		if err == nil {
			connected = database.Ping(db) == nil
		}
	}
	return setupStatusResp{
		Installed:    s.st.Installed,
		DSNSource:    src,
		DSNConnected: connected,
	}, nil
}

type checkEnvResp struct {
	GoVersion      string `json:"go_version"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	DSNSource      string `json:"dsn_source"`
	DSNPresent     bool   `json:"dsn_present"`
	I18nLangs      int    `json:"i18n_langs"`
	I18nKeys       int    `json:"i18n_keys"`
	RedisAddr      string `json:"redis_addr"`
	RedisReachable bool   `json:"redis_reachable"`
	RedisError     string `json:"redis_error,omitempty"`
}

func (s *SetupService) CheckEnv(ctx context.Context, _ json.RawMessage) (any, error) {
	src := "none"
	if os.Getenv("DATABASE_DSN") != "" {
		src = "env"
	} else if s.cfg.Database.DSN != "" {
		src = "config"
	}
	resp := checkEnvResp{
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		DSNSource:  src,
		DSNPresent: s.cfg.Database.DSN != "" || s.st.DSN != "",
		I18nLangs:  len(i18n.AvailableLangs()),
		I18nKeys:   i18n.KeyCount(),
		RedisAddr:  s.cfg.Redis.Addr,
	}
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := pingRedis(probeCtx, s.cfg.Redis.Addr, s.cfg.Redis.DB, s.cfg); err != nil {
		resp.RedisError = err.Error()
	} else {
		resp.RedisReachable = true
	}
	return resp, nil
}

type testDBRequest struct {
	DSN string `json:"dsn"`
}

func (s *SetupService) TestDB(_ context.Context, input json.RawMessage) (any, error) {
	var req testDBRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid json")
	}
	dsn := req.DSN
	if dsn == "" {
		dsn = s.cfg.Database.DSN
		if dsn == "" {
			dsn = s.st.DSN
		}
	}
	if dsn == "" {
		return nil, errs.ValidationKey("setup.dsn_required", "DSN is required")
	}
	db, err := database.Open(dsn)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	if err := database.Ping(db); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

type testRedisRequest struct {
	Addr string `json:"addr"`
	DB   int    `json:"db"`
}

func (s *SetupService) TestRedis(ctx context.Context, input json.RawMessage) (any, error) {
	var req testRedisRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid json")
	}
	if strings.TrimSpace(req.Addr) == "" {
		req.Addr = s.cfg.Redis.Addr
		req.DB = s.cfg.Redis.DB
	}
	if strings.TrimSpace(req.Addr) == "" {
		return nil, errs.ValidationKey("setup.redis_addr_required", "Redis address is required")
	}
	if err := pingRedis(ctx, req.Addr, req.DB, s.cfg); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

func pingRedis(ctx context.Context, addr string, db int, cfg *config.Config) error {
	client := redis.NewClient(&redis.Options{
		Addr: addr, Username: cfg.Redis.Username, Password: cfg.Redis.Password, DB: db,
		DialTimeout: cfg.Redis.DialTimeoutDuration(), ReadTimeout: cfg.Redis.ReadTimeoutDuration(),
		WriteTimeout: cfg.Redis.WriteTimeoutDuration(),
	})
	defer client.Close()
	return client.Ping(ctx).Err()
}

type initRequest struct {
	DSN            string `json:"dsn"`
	TenantName     string `json:"tenant_name"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	RedisAddr      string `json:"redis_addr"`
	RedisDB        int    `json:"redis_db"`
	JobsEnabled    bool   `json:"jobs_enabled"`
	EmbeddedWorker bool   `json:"embedded_worker"`
}

func (s *SetupService) Initialize(ctx context.Context, input json.RawMessage) (any, error) {
	if s.st.Installed {
		return nil, errs.NewKey(errs.CodeSetupInstalled, "setup.already_installed", "system already installed")
	}

	var req initRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid json")
	}
	if req.TenantName == "" || req.Username == "" || req.Password == "" {
		return nil, errs.ValidationKey("common.validation_failed", "tenant_name, username and password required",
			errs.FieldError{Field: "tenant_name", Message: "required", MessageKey: "common.field_required"},
			errs.FieldError{Field: "username", Message: "required", MessageKey: "common.field_required"},
			errs.FieldError{Field: "password", Message: "required", MessageKey: "common.field_required"},
		)
	}
	if err := access.ValidatePassword(req.Password); err != nil {
		return nil, err
	}
	if req.JobsEnabled {
		if strings.TrimSpace(req.RedisAddr) == "" {
			return nil, errs.ValidationKey("setup.redis_addr_required", "Redis address is required")
		}
		if err := pingRedis(ctx, req.RedisAddr, req.RedisDB, s.cfg); err != nil {
			return nil, errs.NewKey(errs.CodeValidation, "setup.redis_unreachable", "Redis is unreachable")
		}
	}

	// 确定 DSN
	dsn := req.DSN
	if dsn == "" {
		dsn = s.cfg.Database.DSN
		if dsn == "" {
			dsn = s.st.DSN
		}
	}
	if dsn == "" {
		return nil, errs.ValidationKey("setup.dsn_required", "DSN is required")
	}

	// 连接数据库
	db, err := database.Open(dsn)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "open database failed", err)
	}
	if err := database.Ping(db); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "ping database failed", err)
	}

	// AutoMigrate
	if err := AutoMigrate(db); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "auto migrate failed", err)
	}

	// 创建初始租户 + 管理员
	if err := s.createInitialData(ctx, db, req.TenantName, req.Username, req.Password); err != nil {
		return nil, err
	}

	// 状态文件成功落盘后再替换内存状态，避免失败请求被误判为已安装。
	now, err := s.persistInstalledState(req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "save state failed", err)
	}

	// 通知内存状态更新，让 SetupGate 中间件立即放行
	if s.onInstalled != nil {
		s.onInstalled()
	}

	logging.Info(ctx, "setup completed",
		"tenant_name", req.TenantName,
		"username", req.Username,
	)

	return map[string]any{
		"installed":       true,
		"installed_at":    now.Format(time.RFC3339),
		"jobs_enabled":    req.JobsEnabled,
		"embedded_worker": req.JobsEnabled && req.EmbeddedWorker,
	}, nil
}

func (s *SetupService) persistInstalledState(req initRequest) (time.Time, error) {
	now := time.Now().UTC()
	next := *s.st
	next.Installed = true
	next.InstalledAt = now
	if req.DSN != "" {
		next.DSN = req.DSN // 仅当用户手动填写时才写入（env 注入的不落盘）
	}
	enabled := req.JobsEnabled
	next.Jobs.Enabled = &enabled
	next.Jobs.EmbeddedWorker = req.JobsEnabled && req.EmbeddedWorker
	if req.JobsEnabled {
		next.Redis = state.Redis{Addr: strings.TrimSpace(req.RedisAddr), DB: req.RedisDB}
	} else {
		next.Redis = state.Redis{}
	}
	if err := state.Save(s.statePath, &next); err != nil {
		return time.Time{}, err
	}
	*s.st = next
	return now, nil
}

func (s *SetupService) createInitialData(ctx context.Context, db *gorm.DB, tenantName, username, password string) error {
	tenantID := uuid.NewString()
	accountID := uuid.NewString()
	roleID := uuid.NewString()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "hash password failed", err)
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 创建租户
		if err := tx.Create(&tenant.Tenant{
			ID:     tenantID,
			Name:   tenantName,
			Status: status.ActivationEnabled,
		}).Error; err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}

		// 创建管理员账号
		if err := tx.Create(&model.TenantAccount{
			ID:           accountID,
			TenantID:     tenantID,
			Username:     username,
			PasswordHash: string(hash),
			DisplayName:  username,
			Status:       status.ActivationEnabled,
		}).Error; err != nil {
			return fmt.Errorf("create tenant account: %w", err)
		}

		// 创建 admin 角色
		if err := tx.Create(&model.Role{
			ID:        roleID,
			TenantID:  tenantID,
			Code:      "admin",
			Name:      "管理员",
			Status:    status.ActivationEnabled,
			IsBuiltin: true,
		}).Error; err != nil {
			return fmt.Errorf("create role: %w", err)
		}

		// 关联账号-角色
		if err := tx.Create(&model.AccountRole{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			AccountID: accountID,
			RoleID:    roleID,
		}).Error; err != nil {
			return fmt.Errorf("create account_role: %w", err)
		}
		if s.initialDataHook != nil {
			if err := s.initialDataHook(ctx, tx, tenantID, accountID); err != nil {
				return fmt.Errorf("initialize tenant modules: %w", err)
			}
		}

		logging.Info(ctx, "initial data created",
			"tenant_id", tenantID,
			"user_id", accountID,
		)
		return nil
	})
}
