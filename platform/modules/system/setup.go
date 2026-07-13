package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SetupService 安装向导核心服务。
type SetupService struct {
	cfgDSN      string       // 配置/环境变量提供的 DSN
	statePath   string       // state.yaml 路径
	st          *state.State // 当前状态
	onInstalled func()       // 安装完成后的回调（用于更新内存状态）
}

func NewSetupService(cfgDSN, statePath string, st *state.State, onInstalled func()) *SetupService {
	return &SetupService{cfgDSN: cfgDSN, statePath: statePath, st: st, onInstalled: onInstalled}
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
	} else if s.cfgDSN != "" {
		src = "config"
	}
	connected := false
	if s.cfgDSN != "" || s.st.DSN != "" {
		dsn := s.cfgDSN
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
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	DSNSource  string `json:"dsn_source"`
	DSNPresent bool   `json:"dsn_present"`
	I18nLangs  int    `json:"i18n_langs"`
	I18nKeys   int    `json:"i18n_keys"`
}

func (s *SetupService) CheckEnv(_ context.Context, _ json.RawMessage) (any, error) {
	src := "none"
	if os.Getenv("DATABASE_DSN") != "" {
		src = "env"
	} else if s.cfgDSN != "" {
		src = "config"
	}
	return checkEnvResp{
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		DSNSource:  src,
		DSNPresent: s.cfgDSN != "" || s.st.DSN != "",
		I18nLangs:  len(i18n.AvailableLangs()),
		I18nKeys:   i18n.KeyCount(),
	}, nil
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
		dsn = s.cfgDSN
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

type initRequest struct {
	DSN        string `json:"dsn"`
	TenantName string `json:"tenant_name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
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
	if len(req.Password) < 6 {
		return nil, errs.ValidationKey("common.validation_failed", "password must be at least 6 characters",
			errs.FieldError{Field: "password", Message: "min 6", MessageKey: "common.field_min_length"},
		)
	}

	// 确定 DSN
	dsn := req.DSN
	if dsn == "" {
		dsn = s.cfgDSN
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

	// 写入状态文件
	now := time.Now().UTC()
	s.st.Installed = true
	s.st.InstalledAt = now
	if req.DSN != "" {
		s.st.DSN = req.DSN // 仅当用户手动填写时才写入（env 注入的不落盘）
	}
	if err := state.Save(s.statePath, s.st); err != nil {
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
		"installed":    true,
		"installed_at": now.Format(time.RFC3339),
	}, nil
}

func (s *SetupService) createInitialData(ctx context.Context, db *gorm.DB, tenantName, username, password string) error {
	tenantID := uuid.NewString()
	userID := uuid.NewString()
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
			Status: tenant.StatusEnabled,
		}).Error; err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}

		// 创建管理员
		if err := tx.Create(&model.User{
			ID:           userID,
			TenantID:     tenantID,
			Username:     username,
			PasswordHash: string(hash),
			DisplayName:  username,
			Status:       model.StatusEnabled,
		}).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		// 创建 admin 角色
		if err := tx.Create(&model.Role{
			ID:       roleID,
			TenantID: tenantID,
			Code:     "admin",
			Name:     "管理员",
			Status:   model.StatusEnabled,
		}).Error; err != nil {
			return fmt.Errorf("create role: %w", err)
		}

		// 关联用户-角色
		if err := tx.Create(&model.UserRole{
			ID:       uuid.NewString(),
			TenantID: tenantID,
			UserID:   userID,
			RoleID:   roleID,
		}).Error; err != nil {
			return fmt.Errorf("create user_role: %w", err)
		}

		// 分配权限
		perms := []string{
			"demo.hello",
			"demo.echo",
			"system.auth_me",
			"system.auth_permissions",
		}
		for _, p := range perms {
			if err := tx.Create(&model.RolePermission{
				ID:         uuid.NewString(),
				RoleID:     roleID,
				Permission: p,
			}).Error; err != nil {
				return fmt.Errorf("create permission %s: %w", p, err)
			}
		}

		logging.Info(ctx, "initial data created",
			"tenant_id", tenantID,
			"user_id", userID,
		)
		return nil
	})
}
