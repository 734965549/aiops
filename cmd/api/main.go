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
	aicred "github.com/734965549/aiops/internal/ai/infrastructure/credential"
	aiidentity "github.com/734965549/aiops/internal/ai/infrastructure/identity"
	aiobs "github.com/734965549/aiops/internal/ai/infrastructure/observability"
	aipg "github.com/734965549/aiops/internal/ai/infrastructure/persistence"
	aihttp "github.com/734965549/aiops/internal/ai/interfaces/http"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	alertapp "github.com/734965549/aiops/internal/alert/application"
	alertasset "github.com/734965549/aiops/internal/alert/infrastructure/asset"
	alertaudit "github.com/734965549/aiops/internal/alert/infrastructure/audit"
	alertidentity "github.com/734965549/aiops/internal/alert/infrastructure/identity"
	alertinteg "github.com/734965549/aiops/internal/alert/infrastructure/integration"
	alertobs "github.com/734965549/aiops/internal/alert/infrastructure/observability"
	alertpg "github.com/734965549/aiops/internal/alert/infrastructure/persistence"
	alertidem "github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	alerthttp "github.com/734965549/aiops/internal/alert/interfaces/http"
	assetapp "github.com/734965549/aiops/internal/asset/application"
	assetaudit "github.com/734965549/aiops/internal/asset/infrastructure/audit"
	assetinteg "github.com/734965549/aiops/internal/asset/infrastructure/integration"
	assetobs "github.com/734965549/aiops/internal/asset/infrastructure/observability"
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
	inspectionasset "github.com/734965549/aiops/internal/inspection/infrastructure/asset"
	inspectionaudit "github.com/734965549/aiops/internal/inspection/infrastructure/audit"
	inspectionexec "github.com/734965549/aiops/internal/inspection/infrastructure/execution"
	inspectionobs "github.com/734965549/aiops/internal/inspection/infrastructure/observability"
	inspectionpg "github.com/734965549/aiops/internal/inspection/infrastructure/persistence"
	inspectionhttp "github.com/734965549/aiops/internal/inspection/interfaces/http"
	integapp "github.com/734965549/aiops/internal/integration/application"
	integrationdomain "github.com/734965549/aiops/internal/integration/domain"
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
	huaweiobs "github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
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
	migrate := flag.Bool("migrate", false, "run database migrations and exit")
	flag.Parse()

	// -migrate 模式：仅执行数据库迁移后退出，供生产部署流水线在 API 启动前显式调用。
	// docker-compose.prod.yml 推荐方式 A 即使用此模式。
	if *migrate {
		// 先加载配置以获取迁移超时时间，避免创建无超时 context。
		cfg, cfgErr := config.Load(*configPath)
		if cfgErr != nil {
			logger.ReportError("migrate failed", cfgErr)
			os.Exit(1)
		}
		timeout := time.Duration(cfg.Database.MigrateTimeoutS) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := bootstrap.Migrate(ctx, *configPath); err != nil {
			logger.ReportError("migrate failed", err)
			os.Exit(1)
		}
		return
	}

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

	// ---- 凭据加密（AI provider 密钥复用 Integration credential vault）----
	integVault, err := integcred.NewVault(
		app.Cfg.Integration.CredentialEncryptionKey,
		app.Cfg.Integration.CredentialEncryptionKeyVersion,
	)
	if err != nil {
		logger.L().Fatal("init integration credential vault failed", logger.Error(err))
	}
	aiSecretCipher := aicred.NewSecretCipher(
		func(material map[string]string) ([]byte, error) {
			ciphertext, _, err := integVault.Encrypt(integrationdomain.CredentialMaterial(material))
			return ciphertext, err
		},
		func(ciphertext []byte) (map[string]string, error) {
			material, err := integVault.Decrypt(ciphertext)
			if err != nil {
				return nil, err
			}
			return map[string]string(material), nil
		},
	)

	// ---- 装配 AI 工具网关 ----
	outboundPolicy := toolgateway.OutboundPolicy{
		AllowedHosts:  app.Cfg.AI.OutboundAllowedHosts,
		AllowLoopback: app.Cfg.AI.OutboundAllowLoopback || app.Cfg.App.Env == "dev",
	}
	providerRegistry := toolgateway.NewProviderRegistryWithPolicy(outboundPolicy)
	providerRegistry.RegisterExecutor(toolgateway.NewHTTPToolExecutor(toolgateway.ProviderTypeA, "/v1/tools/invoke"))
	// base_url 语义为「含版本段的根地址」（如 OpenAI 的 /v1、方舟的 /api/v3），
	// 执行器只追加 /chat/completions。
	providerRegistry.RegisterExecutor(toolgateway.NewChatCompletionExecutor(toolgateway.ProviderTypeB, "/chat/completions"))
	providerRegistry.RegisterExecutor(toolgateway.NewInternalServiceExecutor(toolgateway.ProviderTypeC, "/api/tool/invoke"))

	providerSvc := aiapp.NewProviderService(aipg.NewProviderRepository(app.DB), providerRegistry, aiSecretCipher)
	if err := providerSvc.Bootstrap(bootCtx, providerSeeds(app.Cfg.AI.Providers)); err != nil {
		logger.L().Fatal("bootstrap ai providers failed", logger.Error(err))
	}

	aiGateway := toolgateway.NewGateway(authorizationSvc, providerRegistry)
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
	analyzeSvc := aiapp.NewAnalyzeService(alertReader, aiGateway, "", aiAuditRecorder)
	analyzeSvc.SetProviderResolver(providerRegistry)
	aiHandler := aihttp.NewHandler(aiGateway, providerSvc, authorizationSvc, analyzeSvc, aiAuditRecorder)

	accessSvc = identityapp.NewAccessControlService(accessRepo, userRepo, identityAuditRecorder)
	identityHandler := identityhttp.NewHandler(userSvc, authSvc, accessSvc, authorizationSvc, loginLimiter, authAuditSvc, loginIPAllowlist)

	// ---- 装配 Asset 限界上下文（Alert §9.1 标签匹配）----
	assetAppRepo := assetpg.NewApplicationRepository(app.DB)
	assetResRepo := assetpg.NewResourceRepository(app.DB)
	assetRuleRepo := assetpg.NewMatchRuleRepository(app.DB)
	assetMatcherSvc := assetapp.NewMatcherService(assetAppRepo, assetResRepo, assetRuleRepo)
	assetRefChecker := assetpg.NewApplicationReferenceChecker(app.DB)
	assetDeleteExecutor := assetpg.NewApplicationDeleteExecutor(app.DB)
	assetSvc := assetapp.NewAssetService(assetAppRepo, assetResRepo, assetRuleRepo, assetRefChecker, assetDeleteExecutor, assetAuditRecorder)
	assetRuleSvc := assetapp.NewMatchRuleService(assetRuleRepo, assetAppRepo, assetResRepo, assetAuditRecorder)

	// ---- 装配 Alert 限界上下文（告警中心 Phase 1：接入/去重/状态流转）----
	alertEventRepo := alertpg.NewAlertEventRepository(app.DB)
	alertSourceRepo := alertpg.NewAlertSourceRepository(app.DB)
	alertSilenceRepo := alertpg.NewAlertSilenceRepository(app.DB)
	alertSvc := alertapp.NewAlertService(alertRepo, alertEventRepo, alertSilenceRepo, alertAuditRecorder)
	assetMatcher := alertasset.NewMatcherAdapter(assetMatcherSvc)
	var idemStore alertidem.Store
	if app.Redis != nil {
		// 幂等等待窗口应明显大于 HTTP 写超时，避免慢 ingest 时重放请求提前超时。
		idemMaxWait := max(time.Duration(app.Cfg.Server.WriteTimeoutS)*time.Second+60*time.Second, 90*time.Second)
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
	integUOW := integpg.NewUnitOfWork(app.DB)
	integAuditRecorder := integaudit.NewRecorder(auditSvc)
	integAccountSvc := integapp.NewAccountService(
		integAccountRepo, integCredentialRepo, integCapabilityRepo, integCheckRepo,
		integVault, integprovider.AllCheckers(), integAuditRecorder, integUOW,
	)
	integHandler := integhttp.NewHandler(integAccountSvc)

	// ---- 装配 Observability 限界上下文 ----
	// huawei_cloud：ak_sk 指标经 integration credential repo + vault 解密后走真实 CES；
	// 其余 huawei 能力与 signoz/prometheus 仍为 fake，供无云密钥环境联调。
	obsEvidenceRepo := obspg.NewEvidenceRepository(app.DB)
	obsAccountAdapter := obsinteg.NewAccountAdapter(integAccountRepo, integCapabilityRepo)
	obsAuditRecorder := obsaudit.NewRecorder(auditSvc)
	huaweiObsCreds := huaweiobs.NewCredentialProvider(integCredentialRepo, integVault)
	obsQuerySvc := obsapp.NewQueryService(
		obsAccountAdapter,
		obsprovider.DefaultRegistry(huaweiObsCreds),
		obsEvidenceRepo,
		obsAuditRecorder,
	)
	obsHandler := obshttp.NewHandler(obsQuerySvc)

	// AnalyzeService 早于 Observability 构造，用 setter 注入只读指标证据端口。
	aiEvidenceCollector := aiobs.NewCollectorAdapter(
		obsQuerySvc,
		obsAccountAdapter,
		aiidentity.NewAccountAccessAuthorizer(authorizationSvc),
		app.Cfg.AI.EvidenceTimeout(),
	)
	analyzeSvc.SetMetricEvidence(aiEvidenceCollector)

	// ---- 装配 CES 告警拉取（Alert 端口 -> Observability 适配 -> 共享租约）----
	alertSyncLeaseRepo := alertpg.NewAlertSyncLeaseRepository(app.DB)
	sourceSvc.SetAccountReader(alertinteg.NewAccountReader(integAccountSvc))
	sourceSvc.SetAccountAccessChecker(alertidentity.NewAccountAccessChecker(authorizationSvc))
	sourceSvc.SetLeaseStore(alertSyncLeaseRepo)
	sourceSvc.SetAllowFakeAccounts(app.Cfg.App.Env != "prod")
	alertPullAdapter := alertobs.NewPullAdapter(obsQuerySvc)
	alertSyncSvc := alertapp.NewAlertSyncService(
		alertSourceRepo,
		alertPullAdapter,
		ingestSvc,
		alertSyncLeaseRepo,
		sourceSvc,
		alertAuditRecorder,
		alertSyncInstanceID(),
	)
	alertHandler.SetSyncService(alertSyncSvc)

	assetSyncBatchRepo := assetpg.NewSyncBatchRepository(app.DB)
	assetDiscoveryAdapter := assetobs.NewDiscoveryAdapter(obsQuerySvc)
	assetIntegAdapter := assetinteg.NewAccountAdapter(integAccountRepo, integCapabilityRepo)
	// 注入进程级 context 与数据范围授权端口，使用单一强制依赖构造器完成装配。
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()
	assetSyncSvc := assetapp.NewSyncService(
		assetAppRepo,
		assetResRepo,
		assetSyncBatchRepo,
		assetDiscoveryAdapter,
		assetIntegAdapter,
		assetAuditRecorder,
	)
	assetSyncSvc.SetLifecycle(rootCtx)
	assetHandler := assethttp.NewHandler(assetSvc, assetRuleSvc, assetSyncSvc)

	var alertSyncWorker *alertapp.AlertSyncWorker
	if app.Cfg.AlertSync.WorkerEnabled {
		alertSyncWorker = alertapp.NewAlertSyncWorker(alertSourceRepo, alertSyncSvc, app.Cfg.AlertSync.ScanInterval())
		alertSyncWorker.Start(rootCtx)
		logger.L().Info("alert sync worker started",
			logger.String("owner_id", alertSyncInstanceID()),
			logger.Int("scan_interval_s", int(app.Cfg.AlertSync.ScanInterval().Seconds())),
		)
	}

	// ---- 装配 Inspection 限界上下文（巡检策略/运行/发现/建议 + 证据链分析）----
	inspectionPolicyRepo := inspectionpg.NewPolicyRepository(app.DB)
	inspectionRunRepo := inspectionpg.NewRunRepository(app.DB)
	inspectionFindingRepo := inspectionpg.NewFindingRepository(app.DB)
	inspectionRecRepo := inspectionpg.NewRecommendationRepository(app.DB)
	inspectionArtifactUOW := inspectionpg.NewArtifactUnitOfWork(app.DB)
	inspectionAuditRecorder := inspectionaudit.NewRecorder(auditSvc)
	inspectionObsAdapter := inspectionobs.NewQueryAdapter(obsQuerySvc)
	inspectionAnalyzer := inspectionapp.NewEvidenceAnalyzer(inspectionObsAdapter)
	inspectionAppCatalog := inspectionasset.NewApplicationCatalogAdapter(assetAppRepo)
	inspectionPolicySvc := inspectionapp.NewPolicyService(inspectionPolicyRepo, inspectionAppCatalog, inspectionAuditRecorder)
	inspectionRunSvc := inspectionapp.NewRunService(
		inspectionPolicyRepo, inspectionRunRepo, inspectionFindingRepo, inspectionRecRepo,
		inspectionAnalyzer, inspectionAuditRecorder, inspectionArtifactUOW,
	)
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

	// 1. 取消进程级 context：在途后台同步 goroutine 收到 runCtx 取消，
	//    discovery 循环退出，finalize 用独立短 ctx 落终态。
	rootCancel()
	// 2. 停止接收新 HTTP 请求并处理在途请求。
	shutdownTimeout := time.Duration(app.Cfg.Server.ShutdownTimeoutS) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}
	httpShutdownCtx, cancelHTTP := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelHTTP()
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		logger.L().Error("server shutdown error", logger.Error(err))
	}
	// 3. 后台同步等待使用独立预算，避免 HTTP drain 吃掉 finalize 时间。
	syncShutdownCtx, cancelSync := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelSync()
	if !assetSyncSvc.WaitContext(syncShutdownCtx) {
		logger.L().Warn("asset sync shutdown wait timed out")
	}
	if alertSyncWorker != nil && !alertSyncWorker.WaitContext(syncShutdownCtx) {
		logger.L().Warn("alert sync worker shutdown wait timed out")
	}
	logger.L().Info("aiops-api stopped")
}

func alertSyncInstanceID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("api:%s:%d", strings.TrimSpace(host), os.Getpid())
}

func providerSeeds(providers []config.AIProviderConfig) []aiapp.ProviderSeed {
	seeds := make([]aiapp.ProviderSeed, 0, len(providers))
	for _, p := range providers {
		seeds = append(seeds, aiapp.ProviderSeed{
			ID:          p.ID,
			Name:        p.Name,
			Type:        p.Type,
			BaseURL:     p.BaseURL,
			Model:       p.Model,
			APIKey:      p.APIKey,
			TimeoutMS:   p.TimeoutMS,
			Headers:     p.Headers,
			Enabled:     p.Enabled,
			Description: p.Description,
		})
	}
	return seeds
}
