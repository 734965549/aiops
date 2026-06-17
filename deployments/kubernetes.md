# Kubernetes 部署说明

本文档面向 staging / production 的 Kubernetes 部署。当前项目的推荐形态是：

```text
Browser
  -> Ingress / Gateway
  -> Frontend static hosting (Nginx / CDN / object storage)
  -> aiops-api Deployment
  -> external PostgreSQL
  -> external Redis
```

数据库和 Redis 默认按外部托管资源处理，不在本仓库的 Kubernetes 清单中创建 PostgreSQL、Redis、PVC 或公网端口。

## 部署边界

- Kubernetes 只部署应用层：`aiops-api`、可选前端静态资源服务、Ingress / Service / ConfigMap / Secret。
- PostgreSQL 外挂：使用 RDS、自建高可用 PostgreSQL、或平台 DBA 提供的实例。
- Redis 外挂：生产环境必须可用，`AIOPS_REDIS__REQUIRED=true`。
- 数据库初始化和迁移必须在 API 发布前由 DBA 显式执行，保持 `AIOPS_DATABASE__AUTO_MIGRATE=false`。
- 不使用 initContainer、应用启动钩子或多副本 API 自动迁移生产数据库，避免并发迁移、权限扩大和不可审计变更。
- 不把数据库密码、JWT secret、Redis 密码、AI provider key 写入 Git、ConfigMap、镜像层或前端环境变量。

## 前置条件

1. 已有可访问的 Kubernetes 集群，业务命名空间建议固定为 `aiops`。
2. 已有外部 PostgreSQL，且从 K8s 节点或 Pod 网络可访问。
3. 已有外部 Redis，生产环境建议开启密码、TLS 或内网访问控制。
4. CI/CD 能构建并推送后端镜像：

   ```bash
   docker build -f deployments/Dockerfile \
     -t registry.example.com/aiops/aiops-api:<version> \
     --build-arg VERSION=<version> \
     --build-arg COMMIT=<commit> \
     --build-arg BUILD_AT=<build-time> \
     .
   docker push registry.example.com/aiops/aiops-api:<version>
   ```

5. 前端已完成生产构建：

   ```bash
   cd web
   npm run build
   ```

   当前仓库没有前端 Dockerfile。生产可选择 Nginx 镜像、对象存储 + CDN，或企业静态资源平台托管 `web/dist`。

## 外挂数据库要求

PostgreSQL：

- 版本建议 PostgreSQL 14+，推荐 16。
- 生产库账号最小权限至少需要连接、建表、建索引、写入迁移元数据和业务表数据。
- `schema_migrations` 按仓库迁移账本结构维护，不能混用 golang-migrate 或其它迁移工具的元数据表。
- 生产建议开启 SSL，按数据库侧要求设置 `AIOPS_DATABASE__SSL_MODE=require` 或 `verify-full`。
- 根据 API 副本数控制连接池：`max_open_conns * replicas` 不应超过数据库连接上限。

Redis：

- 生产设置 `AIOPS_REDIS__REQUIRED=true`，Redis 不可用时 `/readyz` 必须阻止接流量。
- Redis 用于 refresh token 轮换等会话能力，不应视为可随意丢失的本地缓存。
- 如果使用托管 Redis，确认安全组、子网路由、密码、DB 编号和 TLS 策略。

## 发布顺序

```text
1. 构建并推送 aiops-api 镜像
2. 构建并发布前端静态资源
3. 备份外部 PostgreSQL
4. DBA 在外部 PostgreSQL 执行建表、种子数据和迁移账本 SQL
5. apply / update Kubernetes ConfigMap、Secret、Deployment、Service、Ingress
6. 等待 /readyz 返回 data.status=ready
7. 执行业务 E2E 或手工验收
8. 观察日志、审计、告警 15-30 分钟
```

## 数据库初始化

生产 Kubernetes 部署不允许 API 进程自动初始化数据库，也不在 Pod、initContainer 或 K8s Job 中运行迁移。数据库初始化由 DBA 在外部 PostgreSQL 上统一执行，应用只读取结果并通过 `/readyz` 校验状态。

DBA 执行范围包括：

- 按版本顺序执行 `migrations/*.up.sql`。
- 确认所有业务表、权限种子、Runbook 种子和审计相关表已成功创建。
- 执行 `migrations/manual_schema_migrations.sql`，创建并维护 `schema_migrations` 迁移账本表。
- 确认 `schema_migrations` 已记录当前发布版本包含的全部 SQL 版本。

`schema_migrations` 不是业务表，它是应用 readiness 用来判断数据库是否追平当前镜像内 `migrations/` 的账本。即使 DBA 已手工执行了所有 `*.up.sql`，也必须同步写入账本；否则 `/readyz` 会返回 migration pending，Pod 不应接收流量。

DBA SQL 统一目录为仓库根目录下的 `migrations/`。生产初始化按 `migrations/README.md` 中列出的顺序执行：

```text
migrations/0001_init_identity.up.sql
migrations/0002_seed_admin_permissions.up.sql
migrations/0003_external_identity.up.sql
migrations/0004_user_provisioning_permissions.up.sql
migrations/0005_user_role_source.up.sql
migrations/0006_auth_audit.up.sql
migrations/0007_init_alert.up.sql
migrations/0008_init_asset.up.sql
migrations/0009_init_audit.up.sql
migrations/0010_ai_analyze_permission.up.sql
migrations/0011_init_execution.up.sql
migrations/0012_init_runbook.up.sql
migrations/0013_dashboard_permission.up.sql
migrations/0014_init_asset_match_rule.up.sql
migrations/0015_identity_access_control_management.up.sql
migrations/manual_schema_migrations.sql
```

执行完成后检查：

```sql
SELECT version, name, applied_at
FROM public.schema_migrations
ORDER BY version;
```

只有在确认对应版本的 `*.up.sql` 已经实际执行成功后，才能写入该版本账本。不要为了让 `/readyz` 变绿而预写未执行版本。

仓库内的 `go run ./cmd/migrate` / `make migrate` 是开发、测试环境可选方式；在“DBA 统一执行 SQL”的生产模式下，不作为 Kubernetes 部署步骤使用。当前 `deployments/Dockerfile` 也只打包 API 进程，不包含独立迁移二进制。

## 基础清单示例

以下 YAML 是可直接改造的基础清单。生产环境请按企业镜像仓库、Ingress Controller、Secret 管理器和网络策略调整。

### Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: aiops
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aiops-api-config
  namespace: aiops
data:
  AIOPS_APP__NAME: aiops
  AIOPS_APP__ENV: prod
  AIOPS_APP__TIMEZONE: Asia/Shanghai

  AIOPS_SERVER__HOST: 0.0.0.0
  AIOPS_SERVER__PORT: "8080"
  AIOPS_SERVER__READ_TIMEOUT_S: "15"
  AIOPS_SERVER__WRITE_TIMEOUT_S: "30"
  AIOPS_SERVER__SHUTDOWN_TIMEOUT_S: "10"

  AIOPS_LOGGER__LEVEL: info
  AIOPS_LOGGER__FORMAT: json
  AIOPS_LOGGER__OUTPUT: stdout

  AIOPS_DATABASE__DRIVER: postgres
  AIOPS_DATABASE__HOST: prod-postgres.example.internal
  AIOPS_DATABASE__PORT: "5432"
  AIOPS_DATABASE__USER: aiops
  AIOPS_DATABASE__NAME: aiops
  AIOPS_DATABASE__SSL_MODE: require
  AIOPS_DATABASE__MAX_IDLE_CONNS: "10"
  AIOPS_DATABASE__MAX_OPEN_CONNS: "50"
  AIOPS_DATABASE__CONN_MAX_LIFETIME_S: "60"
  AIOPS_DATABASE__LOG_LEVEL: warn
  AIOPS_DATABASE__AUTO_MIGRATE: "false"

  AIOPS_REDIS__REQUIRED: "true"
  AIOPS_REDIS__ADDR: prod-redis.example.internal:6379
  AIOPS_REDIS__DB: "0"
  AIOPS_REDIS__POOL_SIZE: "50"

  AIOPS_AUTH__JWT_ISSUER: aiops
  AIOPS_AUTH__ACCESS_TTL_M: "120"
  AIOPS_AUTH__REFRESH_TTL_H: "168"
  AIOPS_AUTH__BOOTSTRAP_USERNAME: ""
  AIOPS_AUTH__BOOTSTRAP_PASSWORD: ""
  AIOPS_AUTH__BOOTSTRAP_DISPLAY_NAME: ""

  # 分域部署时填写前端正式 origin；同源反代时可不依赖 CORS。
  AIOPS_CORS__ALLOW_ORIGINS: https://aiops.example.com
  AIOPS_CORS__ALLOW_CREDENTIALS: "true"
```

### Secret

生产建议由 External Secrets、Sealed Secrets、Vault、云厂商 Secret Manager 或 CI 注入。下面仅展示字段形态。

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aiops-api-secret
  namespace: aiops
type: Opaque
stringData:
  AIOPS_DATABASE__PASSWORD: "<postgres-password>"
  AIOPS_REDIS__PASSWORD: "<redis-password>"
  AIOPS_AUTH__JWT_SECRET: "<at-least-32-bytes-high-entropy-secret>"
```

如果 Redis 不需要密码，可以删除 `AIOPS_REDIS__PASSWORD`，不要写空密码到共享模板中。

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aiops-api
  namespace: aiops
  labels:
    app.kubernetes.io/name: aiops-api
spec:
  replicas: 2
  revisionHistoryLimit: 5
  selector:
    matchLabels:
      app.kubernetes.io/name: aiops-api
  template:
    metadata:
      labels:
        app.kubernetes.io/name: aiops-api
    spec:
      containers:
        - name: api
          image: registry.example.com/aiops/aiops-api:<version>
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
          envFrom:
            - configMapRef:
                name: aiops-api-config
            - secretRef:
                name: aiops-api-secret
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 15
            timeoutSeconds: 5
            failureThreshold: 5
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 30
            periodSeconds: 20
            timeoutSeconds: 5
            failureThreshold: 3
          lifecycle:
            preStop:
              exec:
                command: ["sh", "-c", "sleep 10"]
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 1000m
              memory: 1Gi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
      terminationGracePeriodSeconds: 30
```

探针语义：

- `/healthz` 是 liveness，只检查进程存活。
- `/readyz` 是 readiness，会检查 config、migration、db、redis；只有顶层 `data.status=ready` 才能接流量。
- 迁移未追平、外部数据库不可达、Redis 必需但不可达时，Pod 应保持未就绪。

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: aiops-api
  namespace: aiops
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: aiops-api
  ports:
    - name: http
      port: 8080
      targetPort: http
```

### Ingress

分域部署示例：`https://api.example.com` 指向 API，前端部署在 `https://aiops.example.com`。

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: aiops-api
  namespace: aiops
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "60"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "60"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - api.example.com
      secretName: aiops-api-tls
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: aiops-api
                port:
                  name: http
```

同源部署时，建议由前端 Nginx 或网关把这些路径反代到 `aiops-api`：

- `/api`
- `/healthz`
- `/readyz`
- `/version`

## 前端部署

### 同源反代

适合企业内网统一域名，例如 `https://aiops.example.com`。构建时 `VITE_API_BASE` 留空或设为 `/`，浏览器请求同源路径。

Nginx 关键配置：

```nginx
server {
  listen 80;
  server_name aiops.example.com;

  root /usr/share/nginx/html;
  index index.html;

  location / {
    try_files $uri $uri/ /index.html;
  }

  location /api/ {
    proxy_pass http://aiops-api.aiops.svc.cluster.local:8080/api/;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }

  location = /healthz {
    proxy_pass http://aiops-api.aiops.svc.cluster.local:8080/healthz;
  }

  location = /readyz {
    proxy_pass http://aiops-api.aiops.svc.cluster.local:8080/readyz;
  }

  location = /version {
    proxy_pass http://aiops-api.aiops.svc.cluster.local:8080/version;
  }
}
```

### 分域部署

适合前端静态资源在 CDN / 对象存储，API 独立域名，例如：

```text
Frontend: https://aiops.example.com
API:      https://api.example.com
```

前端构建：

```bash
VITE_API_BASE=https://api.example.com npm run build
```

后端配置：

```text
AIOPS_CORS__ALLOW_ORIGINS=https://aiops.example.com
AIOPS_CORS__ALLOW_CREDENTIALS=true
```

`allow_credentials=true` 时禁止使用 `*`。

## 部署命令示例

```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl -n aiops rollout status deployment/aiops-api
```

验证：

```bash
kubectl -n aiops get pods
kubectl -n aiops logs deploy/aiops-api --tail=100
curl -fsS https://api.example.com/healthz
curl -fsS https://api.example.com/readyz
curl -fsS https://api.example.com/version
```

`/readyz` 必须返回统一响应，且 `data.status` 为 `ready`。

## E2E 验收

staging 环境建议在迁移和发布后执行：

```powershell
$env:API_BASE = "https://api.example.com"
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
.\scripts\e2e-identity-access.ps1
```

若生产环境不允许自动化写入测试数据，应至少按 `docs/demo-flow.md` 和 `docs/acceptance-checklist.md` 做人工抽检。

## 外挂数据库排障

| 现象 | 常见原因 | 处理 |
| --- | --- | --- |
| Pod Running 但未 Ready | `/readyz` 中 migration/db/redis 非 ok | 查 `kubectl logs` 和 `/readyz` 的 checks |
| `migration` degraded | 外部数据库未执行最新 SQL，或 `schema_migrations` 账本未同步 | 由 DBA 补齐 SQL 与账本后，再滚动 API |
| `db` down | 安全组、DNS、SSL、账号密码或连接池配置错误 | 从临时调试 Pod 或 DBA 运维网络验证连通性 |
| `redis` down | `redis.required=true` 且 Redis 不可达 | 检查 Redis 地址、密码、DB 编号和网络策略 |
| 登录后 403 | 权限种子迁移未追平，常见于缺少 `0002` 或后续权限迁移 | 执行完整迁移并检查 `schema_migrations` |
| API 启动失败 | prod 环境使用弱 JWT、配置非法、bootstrap 绑定失败 | 检查 Secret、关闭 bootstrap、确认迁移完整 |

## 生产安全检查

- `AIOPS_APP__ENV=prod`。
- `AIOPS_DATABASE__AUTO_MIGRATE=false`。
- `AIOPS_AUTH__BOOTSTRAP_USERNAME/PASSWORD` 为空。
- `AIOPS_AUTH__JWT_SECRET` 使用高熵密钥，长度至少 32 字节。
- PostgreSQL / Redis 不通过 Kubernetes LoadBalancer、NodePort 或 Ingress 暴露。
- 前端不持有任何服务端密钥。
- CORS 只允许正式前端域名。
- API 镜像 tag 使用不可变版本，不使用 `latest`。
- 外部 PostgreSQL 已备份，DBA 建表、种子数据和 `schema_migrations` 账本执行结果可追溯。
- 日志使用 stdout/json，便于平台采集并保留 `trace_id`。

## 回滚

API 回滚：

```bash
kubectl -n aiops rollout undo deployment/aiops-api
kubectl -n aiops rollout status deployment/aiops-api
```

前端回滚：切回上一版静态资源目录、镜像 tag 或 CDN 发布版本。

数据库回滚：不要由应用自动执行。按 DBA 流程基于备份和对应 `*.down.sql` 人工评估后处理。若迁移失败，原则上不要继续发布新 API，先修复数据库状态。

## 相关文档

- `deployments/README.md`：Compose 与镜像构建说明。
- `ops/migration-contract.md`：迁移 runner 与生产迁移边界。
- `ops/health-contract.md`：`/healthz` 和 `/readyz` 契约。
- `docs/release-checklist.md`：发布前检查清单。
- `docs/acceptance-checklist.md`：主链路验收顺序。
