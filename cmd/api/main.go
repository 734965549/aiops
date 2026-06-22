// Package main 是 aiops-api 的入口。
//
// 启动流程：
//  1. 通过 bootstrap.Init 准备配置、日志、PG、Redis。
//  2. 装配各限界上下文（仓储 -> 应用服务 -> HTTP handler -> 路由注册器）。
//  3. 创建 Gin 引擎并启动 HTTP 服务，捕获信号优雅退出。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	aialert "github.com/734965549/aiops/internal/ai/infrastructure/alert"
	aiaudit "github.com/734965549/aiops/internal/ai/infrastructure/audit"
	aihttp "github.com/734965549/aiops/internal/ai/interfaces/http"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	alertapp "github.com/734965549/aiops/internal/alert/application"
	alertasset "github.com/734965549/aiops/internal/alert/infrastructure/asset"
	alertaudit "github.com/734965549/aiops/internal/alert/infrastructure/audit"
	alertpg "github.com/734965549/aiops/internal/alert/infrastructure/persistence"
	alertidem "github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	alerthttp "github.com/734965549/aiops/internal/alert/interfaces/http"
	assetapp "github.com/734965549/aiops/internal/asset/application"
	assetaudit "github.com/734965549/aiops/internal/asset/infrastructure/audit"
	assetpg "github.com/734965549/aiops/internal/asset/infrastructure/persistence"
	assethttp "github.com/734965549/aiops/internal/asset/interfaces/http"
	auditapp "github.com/734965549/aiops/internal/audit/application"
	auditpg "github.com/734965549/aiops/internal/audit/infrastructure/persistence"
	audithttp "github.com/734965549/aiops/internal/audit/interfaces/http"
	"github.com/734965549/aiops/internal/bootstrap"
	dashapp "github.com/734965549/aiops/internal/dashboard/application"
	dashinfra "github.com/734965549/aiops/internal/dashboard/infrastructure"
	dashhttp "github.com/734965549/aiops/internal/dashboard/interfaces/http"
	execapp "github.com/734965549/aiops/internal/execution/application"
	execalert "github.com/734965549/aiops/internal/execution/infrastructure/alert"
	execaudit "github.com/734965549/aiops/internal/execution/infrastructure/audit"
	execpg "github.com/734965549/aiops/internal/execution/infrastructure/persistence"
	exechttp "github.com/734965549/aiops/internal/execution/interfaces/http"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	identityaudit "github.com/734965549/aiops/internal/identity/infrastructure/audit"
	identityidp "github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	identityldapsession "github.com/734965549/aiops/internal/identity/infrastructure/ldapsession"
	identityoauthstate "github.com/734965549/aiops/internal/identity/infrastructure/oauthstate"
	identitypg "github.com/734965549/aiops/internal/identity/infrastructure/persistence"
	identityhttp "github.com/734965549/aiops/internal/identity/interfaces/http"
	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
	inspectionaudit "github.com/734965549/aiops/internal/inspection/infrastructure/audit"
	inspectionexec "github.com/734965549/aiops/internal/inspection/infrastructure/execution"
	inspectionobs "github.com/734965549/aiops/internal/inspection/infrastructure/observability"
	inspectionpg "github.com/734965549/aiops/internal/inspection/infrastructure/persistence"
	inspectionhttp "github.com/734965549/aiops/internal/inspection/interfaces/http"
	integapp "github.com/734965549/aiops/internal/integration/application"
	integaudit "github.com/734965549/aiops/internal/integration/infrastructure/audit"
	integcred "github.com/734965549/aiops/internal/integration/infrastructure/credential"
	integpg "github.com/734965549/aiops/internal/integration/infrastructure/persistence"
	integprovider "github.com/734965549/aiops/internal/integration/infrastructure/provider"
	integhttp "github.com/734965549/aiops/internal/integration/interfaces/http"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsaudit "github.com/734965549/aiops/internal/observability/infrastructure/audit"
	obsinteg "github.com/734965549/aiops/internal/observability/infrastructure/integration"
	obspg "github.com/734965549/aiops/internal/observability/infrastructure/persistence"
	obsprovider "github.com/734965549/aiops/internal/observability/infrastructure/provider"
	obshttp "github.com/734965549/aiops/internal/observability/interfaces/http"
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	rbalert "github.com/734965549/aiops/internal/runbook/infrastructure/alert"
	rbaudit "github.com/734965549/aiops/internal/runbook/infrastructure/audit"
	rbexec "github.com/734965549/aiops/internal/runbook/infrastructure/execution"
	rbpg "github.com/734965549/aiops/internal/runbook/infrastructure/persistence"
	rbhttp "github.com/734965549/aiops/internal/runbook/interfaces/http"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/internal/version"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/logger"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ./configs/config.yaml)")
	flag.Parse()

	bootTimeout := 30 * time.Second
	if cfg, err := config.Load(*configPath); err == nil && cfg.App.BootstrapTimeoutS > 0 {
		bootTimeout = time.Duration(cfg.App.BootstrapTimeoutS) * time.Second
	}
	bootCtx, bootCancel := context.WithTimeout(context.Background(), bootTimeout)
	defer bootCancel()

	app, err := bootstrap.Init(bootCtx, *configPath)
	if err != nil {
		logger.ReportError("bootstrap failed", err)
		os.Exit(1)
	}
	defer app.Close()
	startedAt := time.Now()

	logger.L().Info("aiops-api starting",
		logger.String("version", version.Get().Version),
		logger.String("commit", version.Get().Commit),
		logger.String("build_at", version.Get().BuildAt),
	)

	// ---- 装配 JWT / Authenticator ----
	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     app.Cfg.Auth.JWTSecret,
		Issuer:     app.Cfg.Auth.JWTIssuer,
		AccessTTL:  app.Cfg.Auth.AccessTTL(),
		RefreshTTL: app.Cfg.Auth.RefreshTTL(),
	})
	if err != nil {
		logger.L().Fatal("init jwt manager failed", logger.Error(err))
	}
	authenticator := auth.NewJWTAuthenticator(jwtMgr)

	// ---- 装配 Identity 限界上下文 ----
	userRepo := identitypg.NewUserRepository(app.DB)
	externalIDRepo := identitypg.NewExternalIdentityRepository(app.DB)
	authAuditRepo := identitypg.NewAuthAuditRepository(app.DB)
	accessRepo := identitypg.NewAccessControlRepository(app.DB, app.Redis, app.Cfg.Auth.GrantCacheTTL())
	userSvc := identityapp.NewUserService(userRepo)
	if app.Redis == nil {
		if app.Cfg.Redis.Required {
			logger.L().Fatal("redis is required but unavailable; refresh token rotation needs session store")
		}
		logger.L().Warn("redis unavailable: refresh token rotation disabled (dev only, never use in prod)")
	}
	var refreshStore auth.RefreshTokenStore = auth.NoopRefreshTokenStore{}
	if app.Redis != nil {
		refreshStore = auth.NewRedisRefreshTokenStore(app.Redis)
	}
	identityProviders, err := identityidp.BuildRegistryFromConfig(app.Cfg.App.Env, app.Cfg.Identity.Providers)
	if err != nil {
		logger.L().Fatal("build identity providers failed", logger.Error(err))
	}
	var ldapBrowseStore identityldapsession.Store = identityldapsession.NewMemoryStore()
	var oauthStateStore identityoauthstate.Store = identityoauthstate.NewMemoryStore()
	if app.Redis != nil {
		ldapBrowseStore = identityldapsession.NewRedisStore(app.Redis)
		oauthStateStore = identityoauthstate.NewRedisStore(app.Redis)
	}
	authSvc := identityapp.NewAuthService(userRepo, jwtMgr, refreshStore, accessRepo, externalIDRepo, identityProviders, ldapBrowseStore, oauthStateStore, app.Cfg.App.Env)
	loginLimiter := auth.NewLoginAttemptLimiter(auth.LoginRateLimitConfig{
		Enabled:                       app.Cfg.Auth.LoginRateLimit.Enabled,
		IPRequestsPerWindow:           app.Cfg.Auth.LoginRateLimit.IPRequestsPerWindow,
		IPWindowS:                     app.Cfg.Auth.LoginRateLimit.IPWindowS,
		IPFailuresBeforeLockout:       app.Cfg.Auth.LoginRateLimit.IPFailuresBeforeLockout,
		UsernameFailuresBeforeLockout: app.Cfg.Auth.LoginRateLimit.UsernameFailuresBeforeLockout,
		LockoutS:                      app.Cfg.Auth.LoginRateLimit.LockoutS,
	}, app.Redis)
	loginIPAllowlist, err := auth.NewIPAllowlist(app.Cfg.Auth.LoginIPAllowlist)
	if err != nil {
		logger.L().Fatal("init login ip allowlist failed", logger.Error(err))
	}
	accessSvc := identityapp.NewAccessControlService(accessRepo, userRepo, nil)
	authorizationSvc := identityapp.NewAuthorizationService(accessRepo)
	authAuditSvc := identityapp.NewAuthAuditService(authAuditRepo)
	if err := authSvc.EnsureBootstrapUser(
		bootCtx,
		app.Cfg.Auth.BootstrapUsername,
		app.Cfg.Auth.BootstrapPassword,
		app.Cfg.Auth.BootstrapDisplayName,
	); err != nil {
		logger.L().Fatal("ensure bootstrap user failed", logger.Error(err))
	}
	if err := accessSvc.EnsureBootstrapUserRoleByUsername(bootCtx, userRepo, app.Cfg.Auth.BootstrapUsername, "admin"); err != nil {
		logger.L().Fatal("ensure bootstrap admin role failed", logger.Error(err))
	}

	// ---- 装配 AI 工具网关 ----
	outboundPolicy := toolgateway.OutboundPolicy{
		AllowedHosts:  app.Cfg.AI.OutboundAllowedHosts,
		AllowLoopback: app.Cfg.AI.OutboundAllowLoopback || app.Cfg.App.Env == "dev",
	}
	providerRegistry := toolgateway.NewProviderRegistryWithPolicy(outboundPolicy)
	providerRegistry.RegisterExecutor(toolgateway.NewHTTPToolExecutor(toolgateway.ProviderTypeA, "/v1/tools/invoke"))
	providerRegistry.RegisterExecutor(toolgateway.NewOpenAICompatibleExecutor(toolgateway.ProviderTypeB, "/v1/chat/completions"))
	providerRegistry.RegisterExecutor(toolgateway.NewInternalServiceExecutor(toolgateway.ProviderTypeC, "/api/tool/invoke"))

	if err := seedProvidersFromConfig(providerRegistry, app.Cfg.AI.Providers); err != nil {
		logger.L().Fatal("seed ai providers from config failed", logger.Error(err))
	}

	aiGateway := toolgateway.NewGateway(authorizationSvc, providerRegistry)
	defaultAIProviderID := firstEnabledProviderID(app.Cfg.AI.Providers)
	alertRepo := alertpg.NewAlertRepository(app.DB)

	// ---- 装配 Audit 限界上下文（Alert §9.4 操作审计）----
	auditRepo := auditpg.NewOperationAuditRepository(app.DB)
	auditSvc := auditapp.NewOperationAuditService(auditRepo)
	auditHandler := audithttp.NewHandler(auditSvc)
	alertAuditRecorder := alertaudit.NewRecorder(auditSvc)
	assetAuditRecorder := assetaudit.NewRecorder(auditSvc)
	aiAuditRecorder := aiaudit.NewRecorder(auditSvc)
	identityAuditRecorder := identityaudit.NewRecorder(auditSvc)

	alertReader := aialert.NewReaderAdapter(alertRepo)
	analyzeSvc := aiapp.NewAnalyzeService(alertReader, aiGateway, defaultAIProviderID, aiAuditRecorder)
	aiHandler := aihttp.NewHandler(aiGateway, providerRegistry, authorizationSvc, analyzeSvc, aiAuditRecorder)

	accessSvc = identityapp.NewAccessControlService(accessRepo, userRepo, identityAuditRecorder)
	identityHandler := identityhttp.NewHandler(userSvc, authSvc, accessSvc, authorizationSvc, loginLimiter, authAuditSvc, loginIPAllowlist)

	// ---- 装配 Asset 限界上下文（Alert §9.1 标签匹配）----
	assetAppRepo := assetpg.NewApplicationRepository(app.DB)
	assetResRepo := assetpg.NewResourceRepository(app.DB)
	assetRuleRepo := assetpg.NewMatchRuleRepository(app.DB)
	assetMatcherSvc := assetapp.NewMatcherService(assetAppRepo, assetResRepo, assetRuleRepo)
	assetSvc := assetapp.NewAssetService(assetAppRepo, assetResRepo, assetRuleRepo, assetAuditRecorder)
	assetRuleSvc := assetapp.NewMatchRuleService(assetRuleRepo, assetAppRepo, assetResRepo, assetAuditRecorder)
	assetHandler := assethttp.NewHandler(assetSvc, assetRuleSvc)

	// ---- 装配 Alert 限界上下文（告警中心 Phase 1：接入/去重/状态流转）----
	alertEventRepo := alertpg.NewAlertEventRepository(app.DB)
	alertSourceRepo := alertpg.NewAlertSourceRepository(app.DB)
	alertSilenceRepo := alertpg.NewAlertSilenceRepository(app.DB)
	alertSvc := alertapp.NewAlertService(alertRepo, alertEventRepo, alertSilenceRepo, alertAuditRecorder)
	assetMatcher := alertasset.NewMatcherAdapter(assetMatcherSvc)
	var idemStore alertidem.Store
	if app.Redis != nil {
		// 幂等等待窗口应明显大于 HTTP 写超时，避免慢 ingest 时重放请求提前超时。
		idemMaxWait := time.Duration(app.Cfg.Server.WriteTimeoutS)*time.Second + 60*time.Second
		if idemMaxWait < 90*time.Second {
			idemMaxWait = 90 * time.Second
		}
		idemStore = alertidem.NewRedisStore(app.Redis, alertidem.Config{
			DefaultMaxWait: idemMaxWait,
		})
	} else if app.Cfg.Redis.Required {
		logger.L().Fatal("redis is required for alert webhook ingest idempotency (multi-pod X-Request-ID dedup)")
	} else {
		logger.L().Warn("redis unavailable: alert webhook idempotency uses in-process memory store (dev only, never use in prod)")
		idemStore = alertidem.NewMemoryStore()
	}
	ingestSvc := alertapp.NewIngestService(alertRepo, alertEventRepo, alertSourceRepo, assetMatcher, idemStore, alertAuditRecorder)
	sourceSvc := alertapp.NewSourceService(alertSourceRepo, alertAuditRecorder)
	alertHandler := alerthttp.NewHandler(alertSvc, sourceSvc)
	alertIngestHandler := alerthttp.NewIngestHandler(ingestSvc)

	// ---- 装配 Execution 限界上下文（告警处置执行任务）----
	execTaskRepo := execpg.NewTaskRepository(app.DB)
	execStepRepo := execpg.NewStepRepository(app.DB)
	execMediumRepo := execpg.NewMediumRepository(app.DB)
	execCommandSpecRepo := execpg.NewCommandSpecRepository(app.DB)
	execAgentRepo := execpg.NewAgentRepository(app.DB)
	execLeaseRepo := execpg.NewLeaseRepository(app.DB)
	execLogRepo := execpg.NewLogStreamRepository(app.DB)
	execAlertAdapter := execalert.NewAdapter(alertRepo, alertSvc)
	execAuditRecorder := execaudit.NewRecorder(auditSvc)

	// ---- 装配 Runbook 限界上下文（预案模板化处置）----
	rbTemplateRepo := rbpg.NewTemplateRepository(app.DB)
	rbStepRepo := rbpg.NewStepRepository(app.DB)
	rbAuditRecorder := rbaudit.NewRecorder(auditSvc)
	rbAlertAdapter := rbalert.NewAdapter(alertRepo)
	rbSvc := rbapp.NewTemplateService(rbTemplateRepo, rbStepRepo, rbAlertAdapter, rbAuditRecorder)
	rbHandler := rbhttp.NewHandler(rbSvc)
	rbExecutionAdapter := rbexec.NewAdapter(rbSvc)

	execSvc := execapp.NewTaskService(execTaskRepo, execStepRepo, execTaskRepo, execAlertAdapter, execAlertAdapter, execAuditRecorder, rbExecutionAdapter, execMediumRepo, execCommandSpecRepo)
	execMediumSvc := execapp.NewMediumService(execMediumRepo, execAuditRecorder)
	execCommandSpecSvc := execapp.NewCommandSpecService(execCommandSpecRepo)
	execAgentSvc := execapp.NewAgentService(execAgentRepo, execMediumRepo, execAuditRecorder)
	execDispatchSvc := execapp.NewDispatchService(execTaskRepo, execStepRepo, execLeaseRepo, execLogRepo, execCommandSpecRepo, execMediumRepo, execAuditRecorder, app.Cfg.Execution.LeaseTTLSecondsOrDefault())
	execHandler := exechttp.NewHandler(execSvc, execMediumSvc, execCommandSpecSvc, execDispatchSvc)
	execAgentHandler := exechttp.NewAgentHandler(execAgentSvc, execDispatchSvc, app.Cfg.Execution.AgentRegisterTokenOrDefault(app.Cfg.App.Env))

	dashboardStats := &dashinfra.RepoStatsReader{
		Alerts: alertRepo, Tasks: execTaskRepo, Apps: assetAppRepo, Resources: assetResRepo, Templates: rbTemplateRepo,
	}
	dashboardSvc := dashapp.NewSummaryService(dashboardStats)
	dashboardHandler := dashhttp.NewHandler(dashboardSvc)

	// ---- 装配 Integration 限界上下文（云账号/观测平台接入）----
	integAccountRepo := integpg.NewAccountRepository(app.DB)
	integCredentialRepo := integpg.NewCredentialRepository(app.DB)
	integCapabilityRepo := integpg.NewCapabilityRepository(app.DB)
	integCheckRepo := integpg.NewCheckResultRepository(app.DB)
	integVault, err := integcred.NewVault(
		app.Cfg.Integration.CredentialEncryptionKey,
		app.Cfg.Integration.CredentialEncryptionKeyVersion,
	)
	if err != nil {
		logger.L().Fatal("init integration credential vault failed", logger.Error(err))
	}
	integUOW := integpg.NewUnitOfWork(app.DB)
	integAuditRecorder := integaudit.NewRecorder(auditSvc)
	integAccountSvc := integapp.NewAccountService(
		integAccountRepo, integCredentialRepo, integCapabilityRepo, integCheckRepo,
		integVault, integprovider.AllCheckers(), integAuditRecorder, integUOW,
	)
	integHandler := integhttp.NewHandler(integAccountSvc)

	// ---- 装配 Observability 限界上下文（Provider Port + fake adapter）----
	obsEvidenceRepo := obspg.NewEvidenceRepository(app.DB)
	obsAccountAdapter := obsinteg.NewAccountAdapter(integAccountRepo, integCredentialRepo, integCapabilityRepo, integVault)
	obsAuditRecorder := obsaudit.NewRecorder(auditSvc)
	obsQuerySvc := obsapp.NewQueryService(obsAccountAdapter, obsprovider.DefaultFakeRegistry(), obsEvidenceRepo, obsAuditRecorder)
	obsHandler := obshttp.NewHandler(obsQuerySvc)

	// ---- 装配 Inspection 限界上下文（巡检策略/运行/发现/建议 + 证据链分析）----
	inspectionPolicyRepo := inspectionpg.NewPolicyRepository(app.DB)
	inspectionRunRepo := inspectionpg.NewRunRepository(app.DB)
	inspectionFindingRepo := inspectionpg.NewFindingRepository(app.DB)
	inspectionRecRepo := inspectionpg.NewRecommendationRepository(app.DB)
	inspectionArtifactUOW := inspectionpg.NewArtifactUnitOfWork(app.DB)
	inspectionAuditRecorder := inspectionaudit.NewRecorder(auditSvc)
	inspectionObsAdapter := inspectionobs.NewQueryAdapter(obsQuerySvc)
	inspectionAnalyzer := inspectionapp.NewEvidenceAnalyzer(inspectionObsAdapter)
	inspectionPolicySvc := inspectionapp.NewPolicyService(inspectionPolicyRepo, inspectionAuditRecorder)
	inspectionRunSvc := inspectionapp.NewRunService(
		inspectionPolicyRepo, inspectionRunRepo, inspectionFindingRepo, inspectionRecRepo,
		inspectionAnalyzer, inspectionAuditRecorder,
	)
	inspectionRunSvc.SetArtifactUnitOfWork(inspectionArtifactUOW)
	inspectionExecAdapter := inspectionexec.NewAdapter(execSvc)
	inspectionRecSvc := inspectionapp.NewRecommendationService(inspectionRecRepo, inspectionExecAdapter, inspectionAuditRecorder)
	inspectionHandler := inspectionhttp.NewHandler(inspectionPolicySvc, inspectionRunSvc, inspectionRecSvc)

	registrars := []server.RouteRegistrar{
		identityhttp.NewRegistrar(identityHandler, authorizationSvc),
		aihttp.NewRegistrar(aiHandler, authorizationSvc),
		assethttp.NewRegistrar(assetHandler, authorizationSvc),
		audithttp.NewRegistrar(auditHandler, authorizationSvc),
		alerthttp.NewRegistrar(alertHandler, alertIngestHandler, authorizationSvc),
		exechttp.NewRegistrar(execHandler, execAgentHandler, authorizationSvc),
		rbhttp.NewRegistrar(rbHandler, authorizationSvc),
		dashhttp.NewRegistrar(dashboardHandler, authorizationSvc),
		integhttp.NewRegistrar(integHandler, authorizationSvc),
		obshttp.NewRegistrar(obsHandler, authorizationSvc),
		inspectionhttp.NewRegistrar(inspectionHandler, authorizationSvc),
	}

	engine := server.NewEngine(server.Options{
		Cfg:           app.Cfg,
		DB:            app.DB,
		Redis:         app.Redis,
		MigrationDir:  app.MigrationDir,
		Authenticator: authenticator,
		Registrars:    registrars,
		StartedAt:     startedAt,
	})

	srv := server.New(app.Cfg.Server, engine)

	// 启动 server（后台 goroutine），主 goroutine 监听信号。
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sigCh:
		logger.L().Info("signal received, shutting down", logger.String("signal", s.String()))
	case err := <-errCh:
		if err != nil {
			logger.L().Error("http server exited with error", logger.Error(err))
		}
	}

	shutdownTimeout := time.Duration(app.Cfg.Server.ShutdownTimeoutS) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.L().Error("server shutdown error", logger.Error(err))
	}
	logger.L().Info("aiops-api stopped")
}

func seedProvidersFromConfig(registry *toolgateway.ProviderRegistry, providers []config.AIProviderConfig) error {
	for _, p := range providers {
		if strings.TrimSpace(p.ID) == "" {
			continue
		}
		if err := registry.UpsertProvider(toolgateway.ProviderConfig{
			ID:          p.ID,
			Name:        p.Name,
			Type:        toolgateway.ProviderType(p.Type),
			BaseURL:     p.BaseURL,
			APIKey:      p.APIKey,
			TimeoutMS:   p.TimeoutMS,
			Headers:     p.Headers,
			Enabled:     p.Enabled,
			Description: p.Description,
		}); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
	}
	return nil
}

func firstEnabledProviderID(providers []config.AIProviderConfig) string {
	for _, p := range providers {
		if strings.TrimSpace(p.ID) == "" || !p.Enabled {
			continue
		}
		return strings.TrimSpace(p.ID)
	}
	return ""
}
