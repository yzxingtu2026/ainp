package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/asynqadapter"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/idempotency"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/statemachine"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"gorm.io/gorm"
)

type App struct {
	Config                   *config.Config
	DB                       *gorm.DB
	Tokens                   *auth.TokenService
	Authorization            auth.AuthorizationProvider
	Authenticators           *authentication.Registry
	AuthRateLimiter          authentication.RateLimiter
	Capabilities             *capability.Registry
	Actions                  *action.Registry
	Menus                    *menu.Registry
	PageHelp                 *menu.HelpRegistry
	CodeSets                 *codeset.Registry
	Settings                 *setting.Registry
	SettingValues            *setting.Provider
	SettingChanges           *setting.ChangeRegistry
	RuntimeContextProvider   runtimecontext.Provider
	StateMachines            *statemachine.Registry
	AIProviderTypes          *aimodel.ProviderTypeRegistry
	AgentContracts           *aiagent.Registry
	ContextCompactor         aiconversation.ContextCompactor
	KnowledgeService         aiknowledge.Service
	KnowledgeAccessPolicy    aiknowledge.AccessPolicy
	KnowledgeSourceResolver  aiknowledge.DocumentSourceResolver
	KnowledgeSourceFactory   func(aiknowledge.Service) aiknowledge.DocumentSourceResolver
	KnowledgeParser          aiknowledge.DocumentParser
	KnowledgeChunker         aiknowledge.Chunker
	KnowledgeRetriever       aiknowledge.KnowledgeRetriever
	AgentTenantInitializer   func(context.Context, *gorm.DB, string, string) error
	ToolCatalogInitializer   func(context.Context, string, string) error
	NavigationInitializer    func(context.Context, string, string) error
	ModelResolver            aimodel.Resolver
	AIAssetResolver          aimodelruntime.AssetResolver
	EmbeddingRuntime         aimodelruntime.EmbeddingRuntime
	ToolRegistry             *aitool.Registry
	Storage                  storage.ObjectStore
	AttachmentPolicies       *attachment.PolicyRegistry
	AttachmentResourceLinker attachment.ResourceLinker
	AgentAvatarValidator     attachment.AvatarValidator
	TransferTokens           *attachment.TransferTokenService
	Outbox                   *outbox.Runtime
	Jobs                     *job.Runtime
	Notifications            *notification.DefinitionRegistry
	LogProviders             *logcenter.Registry
	PageContexts             *pagecontext.Registry
	NotificationRenderer     *notification.Renderer
	OnHTTP                   func(engine *gin.Engine)
	Auditor                  audit.Recorder
	Installed                bool
}

func New(cfg *config.Config, db *gorm.DB, auditor audit.Recorder, tokens *auth.TokenService) *App {
	checker := permission.NewChecker()
	codeSets := codeset.NewRegistry()
	_ = codeSets.Register(codeset.ActivationStatus())
	_ = codeSets.Register(codeset.AttachmentStatus())
	_ = codeSets.Register(codeset.AttachmentThumbStatus())
	_ = codeSets.Register(codeset.OutboxStatus())
	_ = codeSets.Register(codeset.DeliveryStatus())
	_ = codeSets.Register(codeset.JobRunStatus())
	_ = codeSets.Register(codeset.NotificationRequestStatus())
	_ = codeSets.Register(codeset.NotificationDeliveryStatus())

	actions := action.NewRegistry(checker, auditor)

	// Wire idempotency executor
	idemRepo := idempotency.NewRepository(db)
	idemExec := idempotency.NewExecutor(idemRepo)
	actions.SetIdempotencyExecutor(idempotency.NewActionAdapter(idemExec))

	return &App{
		Config:               cfg,
		DB:                   db,
		Tokens:               tokens,
		Authenticators:       authentication.NewRegistry(),
		AuthRateLimiter:      authentication.NewMemoryRateLimiter(10, time.Minute),
		Capabilities:         capability.NewRegistry(),
		Actions:              actions,
		Menus:                menu.NewRegistry(),
		PageHelp:             menu.NewHelpRegistry(),
		CodeSets:             codeSets,
		Settings:             setting.NewRegistry(),
		SettingChanges:       setting.NewChangeRegistry(),
		StateMachines:        statemachine.NewRegistry(),
		AttachmentPolicies:   attachment.NewPolicyRegistry(),
		Notifications:        notification.NewDefinitionRegistry(),
		LogProviders:         logcenter.NewRegistry(),
		PageContexts:         pagecontext.NewRegistry(),
		ToolRegistry:         aitool.NewRegistry(),
		NotificationRenderer: notification.NewRenderer(),
		Auditor:              auditor,
		Installed:            true,
	}
}

func (a *App) RegisterCapability(c capability.Capability) error {
	return a.Capabilities.Register(c)
}

func (a *App) RegisterAction(act action.Action) error {
	if _, ok := a.Capabilities.Get(act.Meta.Capability); !ok {
		return fmt.Errorf("capability not registered: %s", act.Meta.Capability)
	}
	if act.Meta.Capability != "demo" {
		if err := action.ValidateAIClassification(act.ID, act.Meta.AI); err != nil {
			return err
		}
	}
	return a.Actions.Register(act)
}

func (a *App) RegisterActionAlias(alias action.Alias) error {
	return a.Actions.RegisterAlias(alias)
}

func (a *App) RegisterMenu(m menu.Menu) error {
	if m.BindAction != "" {
		bound, ok := a.Actions.Get(m.BindAction)
		if !ok {
			return fmt.Errorf("bind_action not registered: %s", m.BindAction)
		}
		if bound.Meta.AccessMode() == action.AccessPermission && m.Type == menu.TypeButton && m.Permission == "" {
			return fmt.Errorf("permission action button requires permission: %s", m.ID)
		}
		if bound.Meta.AccessMode() != action.AccessPermission && m.Permission != "" {
			return fmt.Errorf("%s action menu must not declare permission: %s", bound.Meta.AccessMode(), m.ID)
		}
	}
	return a.Menus.Register(m)
}

func (a *App) RegisterCodeSet(set codeset.CodeSet) error { return a.CodeSets.Register(set) }

func (a *App) RegisterSettingCategory(c setting.Category) error {
	return a.Settings.RegisterCategory(c)
}

func (a *App) RegisterSettingDefinition(def setting.Definition) error {
	return a.Settings.RegisterDefinition(def)
}

func (a *App) RegisterStateMachine(machine statemachine.StateMachine) error {
	return a.StateMachines.Register(machine, a.CodeSets, a.Actions)
}

func (a *App) RegisterNotification(def notification.Definition) error {
	return a.Notifications.Register(def)
}

func (a *App) Run() error {
	if a.Config.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(a.Config.I18n.DefaultLanguage)
	}

	engine := httpserver.New(httpserver.Options{
		Mode:            a.Config.Server.Mode,
		Installed:       &a.Installed,
		DevTokenEnabled: a.Config.Auth.DevTokenEnabled,
		Tokens:          a.Tokens,
		Authorization:   a.Authorization,
		Actions:         a.Actions,
		Menus:           a.Menus,
		CodeSets:        a.CodeSets,
		DB:              a.DB,
		OnHTTP:          a.OnHTTP,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if a.Outbox != nil {
		a.Outbox.Start(ctx)
		defer a.Outbox.Stop()
	}

	if a.Jobs != nil && a.Jobs.Enabled {
		a.Jobs.StartRecovery(ctx, a.Config.Jobs.RecoveryIntervalDuration())
	}

	var worker *asynqadapter.Runner
	if a.Jobs != nil && a.Jobs.Enabled && a.Config.Jobs.EmbeddedWorker {
		worker = asynqadapter.NewRunner(
			asynqadapter.RedisOptFromConfig(a.Config.Redis),
			asynqadapter.WorkerConfigFromJobs(a.Config.Jobs),
			a.DB,
			a.Jobs.Registry,
			a.Jobs.Store,
		)
		worker.Start(ctx)
		logging.Info(ctx, "embedded asynq worker enabled",
			"jobs", len(a.Jobs.Registry.List()),
			"concurrency", a.Config.Jobs.Concurrency,
		)
		defer func() {
			_ = worker.Shutdown(a.Config.Jobs.ShutdownTimeoutDuration())
		}()
	}

	srv := &http.Server{Addr: a.Config.Server.Addr, Handler: engine}
	errCh := make(chan error, 1)
	go func() {
		logging.Info(ctx, "http server listening", "addr", a.Config.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return <-errCh
	case err := <-errCh:
		return err
	}
}
