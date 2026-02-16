# Copilot Chat Conversation Export: Containerization architecture review and proposal

**User:** @kukisama
**Thread URL:** https://github.com/copilot/c/41452d92-8ba0-4c35-9644-f8ddd5de9ca7

## @kukisama

看看这个文档靠谱不靠谱。现在代码是powershell写的完整逻辑，如果我要容器化，有没有可能，在文章最后补充一个容器化可能的例子。用你认为最合适的代码来处理，不局限于powershell，尽量短小精干。精简现有MAUcache的逻辑，供任何其他语言去开发使用。至少完成一个现有代码的逻辑实现。另外需要考虑，作为下载用的web服务器需要能查看日志记录，类似iis的日志。现阶段需要预置iis配置，相对而言还是比较麻烦。
现阶段不要提供完整代码，但是应该包含完整的架构设计和可能的各种预先性问题处理

## Copilot



我先来仔细审查这份文档的准确性，然后为容器化方案补充一个完整的架构设计。

让我先看看实际代码结构，验证文档的描述是否靠谱。Let me first examine the actual repository code to verify the document's accuracy and understand the real implementation.Now let me look at more of the document to understand the existing containerization suggestion and other key parts:Now I have a thorough understanding of the codebase. Let me now provide the review and the containerization architecture supplement.

---

## 文档靠谱程度评估

我仔细对照了实际代码和文档内容，**这份审查文档整体质量相当高，是靠谱的**。具体评价：

### ✅ 准确之处
1. **目录结构** — 与仓库实际文件完全吻合
2. **函数调用关系** — `Main → Get-MAUProductionBuilds → Get-MAUApps → Get-MAUCacheDownloadJobs → Invoke-MAUCacheDownload` 这条链路和代码一致
3. **20个目标应用** — 与 `Get-MAUApps.ps1` 中的 `$targetApps` 列表完全对应
4. **P1 问题（异常静默吞噬）** — 确实如此，`Invoke-HttpClientDownload.ps1` 第72-74行 catch 块仅 `Write-Host` 不抛异常，而 `Invoke-MAUCacheDownload.ps1` 第91-106行的重试逻辑永远不会被触发
5. **P2 问题（CachePath 验证不一致）** — `Save-MAUCollaterals` 会自动创建目录，`Invoke-MAUCacheDownload` 对 CachePath 不存在时直接 throw，确实不一致
6. **数据流和时序图** — CDN URL、Plist 解析链路、HEAD 请求获取元信息等都与代码逻辑吻合

### ⚠️ 小瑕疵（不影响整体靠谱性）
1. **P3 中 `$maupath` 默认值**问题存在，但从代码看默认值 `"C:\inetpub\wwwroot\maucache"` 就是硬编码的，这个风险级别可以再探讨
2. 文档提到 `$builds` 变量"未被使用"（P11），但实际上它作为 `$DeltaFromBuildLimiter` 传入了 `Get-MAUCacheDownloadJobs`，在该函数第50-57行用于过滤 delta 包——**这个判断有误**

**总结：文档可信度 90%+，架构图、时序图、问题分析基本都经得起代码层面的交叉验证。P11 那个"$builds 未使用"的结论是不正确的，这是文档中最明显的错误。**

---

## 11. 容器化架构设计方案

> 以下内容建议作为文档的新增章节，追加在第10章总结之后。

### 11.1 容器化动机与目标

现有方案的核心痛点：
- **IIS 依赖重**：需要 Windows Server + IIS 角色 + MIME 类型配置 + 注册表操作
- **环境隔离差**：脚本直接操作 `C:\inetpub\wwwroot\`，与宿主系统耦合
- **日志能力弱**：依赖 IIS 日志模块，无法灵活查询和持久化
- **部署复杂**：需要 `PrepIISServer.ps1` + `CreateScheduledTask.ps1` 多步配置

容器化目标：**一个 Docker 镜像，同时完成「定时同步 CDN」+「对外提供文件服务」+「结构化访问日志」三件事，`docker run` 一行命令即可运行。**

### 11.2 技术选型

| 组件 | 现有方案 | 容器化方案 | 理由 |
|------|---------|-----------|------|
| 同步引擎 | PowerShell + System.Net.Http | **Go** 编写的单二进制 CLI | 无运行时依赖、交叉编译、原生并发、镜像 < 30MB |
| Web 服务器 | IIS (需配置 MIME/目录浏览) | **Caddy** (或内嵌到 Go 的 `net/http.FileServer`) | 零配置静态文件服务、自动 MIME、自带访问日志 |
| 定时调度 | Windows 计划任务 / systemd timer | **内置 `time.Ticker`** 或容器 entrypoint 中 `crond` | 容器内无需外部调度器 |
| Plist 解析 | PowerShell ConvertFrom-Plist | Go `encoding/xml` | Plist 就是 XML，标准库直接解析 |
| 日志系统 | IIS W3C 日志 | Go `log/slog` 结构化 JSON → stdout | Docker 原生 `docker logs`，可对接 Loki/ELK |
| 配置管理 | 脚本参数硬编码 | **环境变量 + YAML** | 容器最佳实践 |

### 11.3 整体架构图

```mermaid
graph TB
    subgraph "Docker Container (maucache)"
        subgraph "同步引擎 (Go Binary)"
            SYNC["maucache-sync<br/>定时同步进程"]
            FETCH["FetchBuilds()<br/>获取 builds.txt"]
            PARSE["ParsePlist()<br/>解析应用清单"]
            DL["DownloadPackages()<br/>并发下载 .pkg"]
            COLL["SaveCollaterals()<br/>保存编录文件"]
        end

        subgraph "文件服务 (Go net/http 或 Caddy)"
            FILESVR["FileServer<br/>静态文件服务<br/>:80"]
            ACCESSLOG["AccessLogger<br/>中间件<br/>JSON 格式"]
            HEALTH["/healthz 端点"]
            METRICS["/metrics 端点<br/>(可选 Prometheus)"]
        end

        subgraph "存储层 (Volume Mount)"
            CACHE["/data/maucache<br/>pkg + collateral 文件"]
            LOGS["/data/logs<br/>结构化日志"]
        end

        SYNC --> FETCH --> PARSE --> DL --> COLL
        DL --> CACHE
        COLL --> CACHE
        FILESVR --> CACHE
        ACCESSLOG --> LOGS
        FILESVR --> ACCESSLOG
    end

    CDN["Microsoft Office CDN"] -->|"HTTPS"| SYNC
    MAC["Mac 客户端"] -->|"HTTP :80"| FILESVR
    ADMIN["管理员"] -->|"查看日志"| LOGS
    ADMIN -->|"健康检查"| HEALTH
```

### 11.4 容器模块职责设计

#### 11.4.1 同步引擎（核心逻辑精简）

现有 PowerShell 核心流程可以精简为 5 个步骤，每个步骤对应一个 Go 函数：

```
步骤 1: FetchBuilds()
  → GET https://officecdnmac.microsoft.com/pr/{GUID}/MacAutoupdate/builds.txt
  → 返回 []string

步骤 2: FetchAppManifest(appID) 
  → GET {CDN_BASE}/{appID}.xml        → 解析 Plist → 提取 pkg 下载 URL
  → GET {CDN_BASE}/{appID}-chk.xml    → 提取版本号
  → GET {CDN_BASE}/{appID}-history.xml → 提取历史版本（可选，404 忽略）
  → 返回 AppInfo{Version, Packages[]URI, Collaterals[]URI}

步骤 3: PlanDownloads(apps, builds)
  → 过滤 delta 包（仅保留 builds.txt 中存在的 from_version）
  → 对每个 URI 发 HEAD 请求获取 ContentLength + LastModified
  → 与本地缓存比对（文件存在 + 大小匹配）
  → 返回 []DownloadJob{URI, Size, NeedDownload}

步骤 4: ExecuteDownloads(jobs)
  → 并发下载（可配置并发数，默认 4）
  → 先下载到 /tmp/scratch，完成后原子 rename 到 /data/maucache
  → 带重试（指数退避，最多 3 次）

步骤 5: SaveCollaterals(apps)
  → 下载 xml/cat/chk 编录文件到 /data/maucache/
```

#### 11.4.2 Web 文件服务 + 访问日志

替代 IIS 的核心需求：
- **静态文件服务**：直接 `http.FileServer(http.Dir("/data/maucache"))`
- **MIME 类型**：Go 标准库自动处理；额外注册 `.pkg → application/octet-stream`、`.cat → application/vnd.ms-pki.seccat`
- **目录浏览**：`FileServer` 默认支持
- **访问日志**：用 HTTP 中间件拦截每个请求，记录 JSON 格式日志

日志格式设计（兼容 IIS W3C 日志的关键字段）：

```json
{
  "ts": "2026-02-16T10:30:45.123Z",
  "method": "GET",
  "path": "/Microsoft_Word_16.93.25011212_Updater.pkg",
  "status": 200,
  "size": 1048576000,
  "duration_ms": 3245,
  "client_ip": "10.0.1.55",
  "user_agent": "Microsoft AutoUpdate/4.73",
  "referer": "-"
}
```

#### 11.4.3 健康检查与可观测性

| 端点 | 用途 | 响应示例 |
|------|------|---------|
| `GET /healthz` | Docker/K8s 健康检查 | `{"status":"ok","last_sync":"2026-02-16T08:00:00Z","cache_files":142}` |
| `GET /metrics` | Prometheus 指标（可选） | `maucache_sync_duration_seconds`, `maucache_files_total`, `maucache_download_errors_total` |
| `GET /sync/status` | 最近同步状态 | `{"running":false,"last_result":"success","next_run":"2026-02-16T12:00:00Z"}` |

### 11.5 配置管理

通过环境变量 + 可选 YAML 配置文件双重支持：

```yaml
# /etc/maucache/config.yaml（容器内）
sync:
  channel: Production       # Production | Preview | Beta
  interval: 6h              # 同步间隔
  concurrency: 4            # 并发下载数
  retry_max: 3              # 最大重试次数
  retry_backoff: 5s         # 重试退避基数
  include_historic: false   # 是否包含历史版本
  
server:
  listen: ":80"             # 监听地址
  read_timeout: 30s
  write_timeout: 300s       # 大文件传输需要较长超时

storage:
  cache_dir: /data/maucache
  scratch_dir: /tmp/scratch
  log_dir: /data/logs

logging:
  level: info               # debug | info | warn | error
  format: json              # json | text
  access_log: true          # 是否记录访问日志
  access_log_file: /data/logs/access.log
```

等效的环境变量映射：

```
MAUCACHE_SYNC_CHANNEL=Production
MAUCACHE_SYNC_INTERVAL=6h
MAUCACHE_SYNC_CONCURRENCY=4
MAUCACHE_SERVER_LISTEN=:80
MAUCACHE_STORAGE_CACHE_DIR=/data/maucache
MAUCACHE_LOGGING_LEVEL=info
```

### 11.6 Dockerfile 设计

```dockerfile
# --- 构建阶段 ---
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /maucache ./cmd/maucache

# --- 运行阶段 ---
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /maucache /usr/local/bin/maucache

EXPOSE 80
VOLUME ["/data/maucache", "/data/logs"]

HEALTHCHECK --interval=60s --timeout=5s \
  CMD wget -qO- http://localhost/healthz || exit 1

ENTRYPOINT ["maucache"]
CMD ["serve", "--sync"]
```

**镜像特征**：
- 多阶段构建，最终镜像 ≈ 25MB（Alpine + 单 Go 二进制）
- 不需要 PowerShell、.NET、IIS
- `ca-certificates` 确保 HTTPS 连接 Microsoft CDN
- `tzdata` 支持时区配置

### 11.7 docker-compose 示例

```yaml
version: "3.8"
services:
  maucache:
    image: maucache:latest
    build: .
    ports:
      - "80:80"
    volumes:
      - maucache-data:/data/maucache
      - maucache-logs:/data/logs
    environment:
      - MAUCACHE_SYNC_CHANNEL=Production
      - MAUCACHE_SYNC_INTERVAL=6h
      - MAUCACHE_SYNC_CONCURRENCY=4
      - MAUCACHE_LOGGING_LEVEL=info
      - TZ=Asia/Shanghai
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost/healthz"]
      interval: 60s
      timeout: 5s
      retries: 3

volumes:
  maucache-data:
    driver: local
  maucache-logs:
    driver: local
```

### 11.8 Go 代码结构设计

```
cmd/maucache/
├── main.go                 # CLI 入口 (cobra/urfave)
├── serve.go                # serve 子命令：启动 web + 定时同步
└── sync.go                 # sync 子命令：仅执行一次同步

internal/
├── config/
│   └── config.go           # 配置加载（env + yaml）
├── cdn/
│   ├── client.go           # HTTP client（带重试、超时）
│   ├── builds.go           # FetchBuilds()
│   ├── manifest.go         # FetchAppManifest() + ParsePlist()
│   └── plist.go            # XML Plist 解析器
├── sync/
│   ├── planner.go          # PlanDownloads() - 比对缓存生成任务
│   ├── downloader.go       # ExecuteDownloads() - 并发下载 + 原子写入
│   ├── collateral.go       # SaveCollaterals()
│   └── scheduler.go        # 定时调度循环
├── server/
│   ├── server.go           # HTTP 服务器初始化
│   ├── fileserver.go       # 静态文件服务 + MIME 注册
│   ├── middleware.go        # 访问日志中间件
│   ├── health.go           # /healthz, /sync/status
│   └── metrics.go          # /metrics (可选)
└── logging/
    └── logger.go           # slog 配置 (JSON/Text)
```

### 11.9 核心逻辑映射表

| 现有 PowerShell 函数 | Go 对应模块 | 关键改进 |
|---------------------|------------|---------|
| `Get-MAUProductionBuilds` | `cdn.FetchBuilds()` | 带 context 超时 + 重试 |
| `Get-MAUApps` + `Get-MAUApp` | `cdn.FetchAppManifest()` | 并发获取 20 个应用 |
| `ConvertFrom-Plist` | `cdn.ParsePlist()` | `encoding/xml` 直接反序列化 |
| `Get-MAUCacheDownloadJobs` | `sync.PlanDownloads()` | HEAD 请求并发化 |
| `Invoke-MAUCacheDownload` | `sync.ExecuteDownloads()` | `errgroup` 并发 + 原子 rename |
| `Invoke-HttpClientDownload` | `cdn.Client.Download()` | 指数退避重试 + context 取消 |
| `Save-MAUCollaterals` | `sync.SaveCollaterals()` | 统一到同一函数 |
| IIS (`PrepIISServer.ps1`) | `server.FileServer()` | **完全消除** |
| 计划任务 (`CreateScheduledTask.ps1`) | `sync.Scheduler` | **内建调度** |
| IIS 日志 | `server.AccessLogMiddleware` | JSON 结构化 + stdout |

### 11.10 预先性问题分析与对策

#### Q1: Plist XML 解析在 Go 中是否可行？

**可行**。MAU 的 Plist 是标准 Apple XML Plist 格式，其结构有限（`dict`、`array`、`string`、`integer`、`true/false`）。Go 中有两种方式：

- **方式 A**：用 `encoding/xml` 手写递归解析器（约100行），完全可控
- **方式 B**：使用 `howett.net/plist` 库，直接 `plist.Unmarshal` 到 Go struct

推荐 **方式 A**，因为只需支持有限节点类型，避免引入外部依赖，且现有 PowerShell 的 `ConvertFrom-Plist` 逻辑本身就是手写递归，移植直接。

#### Q2: 大文件下载中断怎么办？

现有代码的问题：下载直接写到目标路径，中断会留下不完整文件。

容器化方案：
1. **先写 scratch 再 rename**（现有代码部分实现了，但 `Invoke-HttpClientDownload` 本身没有）
2. 使用 Go 的 `os.CreateTemp` + `os.Rename` 实现原子写入
3. 下载进度和状态写入内存状态，可通过 `/sync/status` 查询
4. 启动时检查 scratch 目录残留文件并清理

#### Q3: 并发下载是否会被 CDN 限流？

Microsoft CDN 对单 IP 有一定的连接限制。对策：
- 默认并发数 4（可配置），不要过高
- 每次下载间隔随机 jitter（100-500ms）
- 遇到 429/503 状态码时指数退避
- 遇到 "response ended prematurely" 时自动重试（与现有逻辑一致）

#### Q4: 容器存储性能如何保证？

MAU 全量缓存约 **30-50GB**，涉及大量大文件 I/O：
- **必须使用 named volume 或 bind mount**，不能用容器可写层
- 推荐 bind mount 到宿主的 SSD 路径
- 文件服务发送大文件时使用 `http.ServeFile`（底层调用 `sendfile` 系统调用，零拷贝）
- Go 默认 buffer 足够大，无需额外优化

#### Q5: 访问日志如何替代 IIS 日志？

| 功能 | IIS W3C 日志 | 容器化方案 |
|------|-------------|-----------|
| 日志输出 | 文件（`C:\inetpub\logs\`) | stdout（Docker 捕获）+ 可选文件 |
| 日志格式 | W3C 扩展格式 | JSON 结构化 |
| 日志轮转 | IIS 内置 | Docker logging driver 或 logrotate |
| 日志查询 | 文本搜索 | `docker logs --since` / `jq` 过滤 / Loki |
| 实时查看 | 需打开文件 | `docker logs -f maucache` |

**日志示例命令**：
```bash
# 实时查看访问日志
docker logs -f maucache 2>&1 | jq 'select(.type=="access")'

# 查看最近1小时的404请求
docker logs --since 1h maucache | jq 'select(.status==404)'

# 统计今天各客户端下载量
docker logs --since 24h maucache | jq -r 'select(.type=="access") | .client_ip' | sort | uniq -c | sort -rn
```

#### Q6: 如何处理 HTTPS？

两种方案：
- **方案 A（推荐）**：前置反向代理（Nginx/Traefik）处理 TLS，容器内 plain HTTP
- **方案 B**：使用 Caddy 做文件服务，自动 ACME 证书
- MAU 客户端默认支持 HTTP（不强制 HTTPS），所以内网部署可以直接 HTTP

#### Q7: 如何平滑迁移？

1. 容器的 `/data/maucache` 可以 bind mount 到现有 IIS 站点目录
2. 先并行运行：IIS 继续服务，容器负责同步
3. 验证文件一致后，将 Mac 客户端的 `ManifestServer` 指向容器地址
4. 关闭 IIS

#### Q8: 多架构支持？

Go 天然支持交叉编译：
- `GOOS=linux GOARCH=amd64` — 标准服务器
- `GOOS=linux GOARCH=arm64` — 树莓派 / ARM 服务器
- Docker buildx 可一次构建多架构镜像

#### Q9: Kubernetes 部署考虑？

```yaml
# 简要示意，非完整 K8s manifest
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: maucache
        image: maucache:latest
        ports: [{containerPort: 80}]
        volumeMounts:
        - name: cache
          mountPath: /data/maucache
        livenessProbe:
          httpGet: {path: /healthz, port: 80}
        resources:
          requests: {memory: "128Mi", cpu: "100m"}
          limits: {memory: "512Mi", cpu: "500m"}
      volumes:
      - name: cache
        persistentVolumeClaim: {claimName: maucache-pvc}
```

- 需要 PVC 持久化缓存（30-50GB）
- 可以部署为 Deployment（单副本）+ Service
- 不建议多副本（同步逻辑无锁，会产生竞争）

#### Q10: 从 PowerShell 调用容器化后的同步？

如果仍希望保留 PowerShell 作为编排入口：

```powershell
# 直接触发容器内的同步（通过 HTTP API）
Invoke-RestMethod -Uri "http://maucache-host/sync/trigger" -Method POST

# 查看同步状态
Invoke-RestMethod -Uri "http://maucache-host/sync/status"
```

### 11.11 实现优先级建议

| 阶段 | 内容 | 工作量估计 |
|------|------|-----------|
| **Phase 1** | Go 实现 `FetchBuilds` + `ParsePlist` + `FetchAppManifest` | 2-3 天 |
| **Phase 2** | Go 实现 `PlanDownloads` + `ExecuteDownloads`（含并发+重试） | 2-3 天 |
| **Phase 3** | Go 实现 `FileServer` + `AccessLogMiddleware` + `/healthz` | 1 天 |
| **Phase 4** | Dockerfile + docker-compose + 配置管理 | 1 天 |
| **Phase 5** | 端到端测试 + 与现有 IIS 方案对比验证 | 1-2 天 |
| **Phase 6**（可选） | Prometheus metrics + Grafana dashboard | 1 天 |

**总计约 7-10 个工作日** 可完成核心功能上线。

### 11.12 容器化方案总结

```mermaid
graph LR
    BEFORE["现有方案<br/>Windows Server<br/>+ IIS<br/>+ PowerShell<br/>+ 计划任务<br/>+ 手动配置"]
    AFTER["容器化方案<br/>Alpine Linux<br/>+ Go 单二进制<br/>+ 内建 HTTP 服务<br/>+ 内建定时同步<br/>+ JSON 日志<br/>+ docker run 一行启动"]
    
    BEFORE -->|"容器化"| AFTER
    
    style BEFORE fill:#f96,stroke:#333
    style AFTER fill:#6f9,stroke:#333
```

| 维度 | 现有方案 | 容器化方案 |
|------|---------|-----------|
| 部署复杂度 | 高（IIS + 计划任务 + MIME 配置） | **低**（`docker run` 一行） |
| 操作系统 | 仅 Windows Server | **任意**（Linux/Windows/macOS） |
| 镜像大小 | N/A（宿主安装） | **≈ 25MB** |
| 内存占用 | PowerShell ≈ 200MB + IIS | **≈ 50-100MB** |
| 日志查看 | 需登录服务器查 IIS 日志文件 | **`docker logs -f`** + jq |
| 下载性能 | 串行 | **并发**（默认 4 路） |
| 扩展性 | 修改 PowerShell 脚本 | **HTTP API + 配置文件** |

## @kukisama

go做web服务器安全么？在这个场景下是否足够使用。另外这里用nginx是否更好？

## Copilot

好问题。让我结合你这个 MAUCache 的**实际场景**来分析，而不是泛泛而谈。

---

## Go `net/http` vs Nginx：在 MAUCache 这个场景下

### 先明确场景特征

| 特征 | MAUCache 实际情况 |
|------|------------------|
| 客户端 | 内网 Mac 机器上的 Microsoft AutoUpdate 进程 |
| 暴露面 | **纯内网**，不对公网开放 |
| 并发量 | 几十到几百台 Mac，远非高并发 |
| 文件特征 | 大文件为主（.pkg 几百MB~1GB），少量小文件（.xml/.cat） |
| 协议要求 | MAU 客户端支持 HTTP，不强制 HTTPS |
| 安全威胁 | 内网环境，无外部攻击者，主要关心**稳定性**而非防攻击 |

### 结论：这个场景下 Nginx 更合适

但原因**不是安全**，而是以下几个实际问题：

#### 1. 大文件传输稳定性

Go `net/http.FileServer` 处理大文件时有一个隐含问题——它在一个 goroutine 里同步地把文件写到连接中。如果客户端慢（比如 Wi-Fi 的 Mac 下载 1GB 的 Word 更新包），这个 goroutine 会被长时间占用。

Nginx 在这方面经过了几十年的打磨：
- **sendfile 系统调用**：直接在内核态把文件发给 socket，不经过用户态拷贝
- **AIO（异步 I/O）**：大文件场景下可以开启异步读取
- **客户端慢速处理**：Nginx 的事件循环模型天生擅长处理慢客户端

> Go 也用了 `sendfile`，但 Nginx 对连接管理、超时控制的颗粒度更细。

#### 2. 访问日志是现成的

你特别提到需要**类似 IIS 的日志**。Nginx 原生就有：

```nginx
log_format maucache '$remote_addr - [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_user_agent" '
                    '$request_time';
access_log /data/logs/access.log maucache;
```

这给你的东西：
- IP、时间、请求路径、状态码、传输字节数、客户端 UserAgent、耗时
- **日志轮转**：配合 `logrotate` 或 Nginx 自带的 `access_log ... buffer=32k flush=5m`
- 查询 Mac 客户端下载了什么、花了多久、有没有失败，**一目了然**

Go 自己实现也行，但等于重新造一个访问日志中间件。不值得。

#### 3. 零配置 MIME 和目录浏览

IIS 需要 `PrepIISServer.ps1` 去手动添加 `.pkg`、`.cat` 的 MIME 类型。Nginx 自带 `/etc/nginx/mime.types`，只需补一行：

```nginx
types {
    application/octet-stream pkg mpkg;
}
```

目录浏览：`autoindex on;` 一行搞定。

#### 4. 安全性对比（在这个场景下）

| 威胁 | Go net/http | Nginx | 评价 |
|------|-------------|-------|------|
| 路径穿越 | `http.FileServer` 会自动 `path.Clean`，安全 | 默认安全 | **两者都没问题** |
| slowloris 攻击 | 需手动设 `ReadHeaderTimeout` | 默认防护 | 内网不太可能遇到 |
| 连接耗尽 | 需手动限制 `MaxConnsPerHost` | 内置 `worker_connections` | 内网不太可能遇到 |
| TLS | 手动配置 | 成熟 | MAU 内网用 HTTP，不需要 |

**关键结论**：在纯内网场景下，Go 的安全性是足够的。但 Nginx 几乎不需要你操心这些。

### 推荐架构：Go 同步 + Nginx 服务

把 Go 用在它最擅长的地方（并发下载、Plist 解析、调度逻辑），把 Nginx 用在它最擅长的地方（稳定地伺候文件 + 日志）：

```mermaid
graph LR
    subgraph "Docker Container"
        subgraph "Go 进程"
            SYNC["maucache-sync<br/>定时同步引擎<br/>· 并发下载 CDN<br/>· Plist 解析<br/>· 增量检测"]
            API["/sync/status<br/>/healthz<br/>:8080 内部端口"]
        end

        subgraph "Nginx 进程"
            NG["Nginx<br/>· 静态文件服务 :80<br/>· 访问日志<br/>· 目录浏览<br/>· sendfile 大文件"]
        end

        CACHE["/data/maucache<br/>共享目录"]

        SYNC -->|"写入文件"| CACHE
        NG -->|"读取文件"| CACHE
    end

    CDN["Microsoft CDN"] -->|"HTTPS"| SYNC
    MAC["Mac 客户端"] -->|"HTTP :80"| NG
```

### 修改后的 Dockerfile

```dockerfile
# --- 构建 Go 同步引擎 ---
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /maucache-sync ./cmd/sync

# --- 运行镜像 ---
FROM nginx:1.27-alpine

# 安装必要工具
RUN apk add --no-cache ca-certificates tzdata supervisor

# 复制 Go 二进制
COPY --from=builder /maucache-sync /usr/local/bin/maucache-sync

# Nginx 配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Supervisor 管理两个进程
COPY supervisord.conf /etc/supervisord.conf

EXPOSE 80
VOLUME ["/data/maucache", "/data/logs"]

HEALTHCHECK --interval=60s --timeout=5s \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

CMD ["supervisord", "-c", "/etc/supervisord.conf"]
```

### Nginx 配置

```nginx name=nginx.conf
server {
    listen 80;
    server_name _;

    root /data/maucache;
    autoindex on;                    # 目录浏览（替代 IIS 目录浏览）

    # MIME 类型补充（替代 PrepIISServer.ps1 的 MIME 配置）
    types {
        application/octet-stream     pkg mpkg;
        application/vnd.ms-pki.seccat cat;
        text/xml                     xml;
    }

    # 大文件传输优化
    sendfile           on;
    tcp_nopush         on;
    tcp_nodelay        on;
    keepalive_timeout  65;
    client_max_body_size 0;          # 无上传限制（纯下载场景）

    # 访问日志 - JSON 格式（方便 jq/Loki 解析）
    log_format json_log escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"user_agent":"$http_user_agent"'
        '}';

    access_log /data/logs/access.log json_log buffer=32k flush=5s;
    error_log  /data/logs/error.log warn;

    # 健康检查（Nginx 自身）
    location = /nginx-health {
        access_log off;
        return 200 "ok";
    }

    # 代理 Go 同步引擎的管理 API
    location /sync/ {
        proxy_pass http://127.0.0.1:8080;
    }
    location = /healthz {
        proxy_pass http://127.0.0.1:8080;
    }
}
```

### Supervisor 配置（一个容器跑两个进程）

```ini name=supervisord.conf
[supervisord]
nodaemon=true
logfile=/dev/null
logfile_maxbytes=0

[program:nginx]
command=nginx -g "daemon off;"
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

[program:maucache-sync]
command=/usr/local/bin/maucache-sync --listen=:8080 --cache-dir=/data/maucache
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
```

### 日志使用示例

```bash
# 实时看谁在下载什么
docker logs -f maucache | jq 'select(.status==200) | "\(.client) → \(.path) (\(.size) bytes)"'

# 找出下载失败的请求
docker logs --since 24h maucache | jq 'select(.status>=400)'

# 统计每个客户端下载了多少数据
docker logs --since 7d maucache | jq -r 'select(.status==200) | .client' | sort | uniq -c | sort -rn

# 看哪些 pkg 被下载最多
docker logs --since 7d maucache | jq -r 'select(.path | endswith(".pkg")) | .path' | sort | uniq -c | sort -rn
```

### 最终对比

| | 纯 Go 方案 | Go + Nginx 方案 | 评价 |
|---|-----------|----------------|------|
| 镜像大小 | ~25MB | ~45MB | 多 20MB，可接受 |
| 大文件稳定性 | 够用但不是最优 | **内核级 sendfile** | Nginx 胜 |
| 访问日志 | 自己写中间件 | **原生支持+可定制格式** | Nginx 胜 |
| 安全性 | 内网足够 | 内网足够+更放心 | 平手 |
| 维护成本 | Go 代码量更大 | **各司其职，代码更少** | Nginx 胜 |
| 复杂度 | 一个进程 | 两个进程+supervisor | Go 略胜 |

**最终建议**：Go 做同步引擎 + Nginx 做文件服务。在你这个场景下，这是**最省事也最稳**的组合。Go 的 `net/http` 不是不安全，而是用它来做静态文件服务属于"能做但没必要"——Nginx 本来就干这个的，何必重新造。

## @kukisama

那是不是意味着，其实是两个容器，一个nginx，一个我的代码？需要关联到某个nginx。或者需要nginx配置，然后我的容器的写文件操作需要写入到特定目录，正好nginx也可以用到？

## Copilot



对，你抓到了关键点。这里有两种方式，各有适合的场景：

## 方案对比

### 方案 A：两个容器（docker-compose）

```mermaid
graph LR
    subgraph "Container 1: maucache-sync"
        GO["Go 同步引擎"]
    end
    subgraph "Container 2: nginx"
        NG["Nginx 文件服务"]
    end
    subgraph "共享 Volume"
        VOL["/data/maucache"]
    end

    CDN["Microsoft CDN"] -->|HTTPS| GO
    GO -->|写文件| VOL
    NG -->|读文件| VOL
    MAC["Mac 客户端"] -->|HTTP :80| NG
```

### 方案 B：单容器（supervisor 管两个进程）

```mermaid
graph LR
    subgraph "Single Container"
        GO["Go 同步引擎"]
        NG["Nginx"]
        VOL["/data/maucache"]
        GO -->|写文件| VOL
        NG -->|读文件| VOL
    end

    CDN["Microsoft CDN"] -->|HTTPS| GO
    MAC["Mac 客户端"] -->|HTTP :80| NG
```

### 哪个更好？

| 维度 | 方案 A：两个容器 | 方案 B：单容器 |
|------|----------------|---------------|
| **部署复杂度** | 需要 docker-compose | `docker run` 一行 |
| **Docker 最佳实践** | ✅ 一容器一进程 | ❌ 需要 supervisor |
| **独立升级** | ✅ 可以单独升 nginx 或同步引擎 | ❌ 一起重建 |
| **故障隔离** | ✅ 同步挂了不影响文件服务 | ❌ 容器挂了全挂 |
| **给用户的感觉** | 稍复杂，要理解两个东西 | 简单，一个东西搞定 |
| **适合场景** | 正式运维环境 | 小团队/快速上手 |

**你这个场景我推荐方案 A**，理由很实际：

1. **同步和服务是两个独立关注点**——同步可能跑 30 分钟在疯狂下载，Nginx 需要一直稳定响应客户端。分开后互不干扰
2. **Nginx 容器直接用官方镜像**——不用自己构建，`nginx:alpine` 拿来就用，安全补丁跟着官方走
3. **你的代码只需要关心一件事**——往目录里写文件，干净利落

## 方案 A 具体实现

核心就一个原则：**两个容器挂载同一个 Volume，一个写，一个读。**

### docker-compose.yaml

```yaml name=docker-compose.yaml
services:
  # ---- 你的代码：只管同步 ----
  sync:
    build: .
    volumes:
      - maucache:/data/maucache       # 同步引擎往这里写文件
      - logs:/data/logs                # 同步日志
    environment:
      - MAUCACHE_SYNC_INTERVAL=6h
      - MAUCACHE_SYNC_CHANNEL=Production
      - MAUCACHE_SYNC_CONCURRENCY=4
      - TZ=Asia/Shanghai
    restart: unless-stopped

  # ---- Nginx：只管对外提供文件 ----
  web:
    image: nginx:1.27-alpine           # 直接用官方镜像，不用自己构建
    ports:
      - "80:80"
    volumes:
      - maucache:/data/maucache:ro     # 只读挂载！Nginx 只需要读
      - logs:/data/logs                # Nginx 访问日志也写到这里
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    depends_on:
      - sync
    restart: unless-stopped

volumes:
  maucache:     # 两个容器共享的缓存目录
  logs:         # 共享的日志目录
```

注意关键点：
- **`maucache` volume 被两个容器同时挂载**：`sync` 可读写，`web` 以 `:ro` 只读挂载
- **Nginx 用的是官方镜像原封不动**，只把你的 `nginx.conf` 映射进去
- `depends_on` 保证 Nginx 启动时 sync 已经在跑了（但 Nginx 不依赖 sync 活着）

### 你的同步引擎 Dockerfile

```dockerfile name=Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /maucache-sync ./cmd/sync

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /maucache-sync /usr/local/bin/maucache-sync

VOLUME ["/data/maucache"]
ENTRYPOINT ["maucache-sync"]
```

这个镜像里**没有 Nginx、没有 supervisor**，干干净净就一个 Go 二进制。它的工作就是：
1. 定时从 CDN 拉文件
2. 写到 `/data/maucache/` 目录
3. 完事

### Nginx 配置

```nginx name=nginx.conf
server {
    listen 80;
    server_name _;
    root /data/maucache;

    # 目录浏览（替代 IIS 目录浏览功能）
    autoindex on;

    # 补充 MIME 类型（替代 PrepIISServer.ps1）
    types {
        application/octet-stream     pkg mpkg;
        application/vnd.ms-pki.seccat cat;
        text/xml                     xml;
    }

    # 大文件优化
    sendfile    on;
    tcp_nopush  on;

    # JSON 格式访问日志（替代 IIS W3C 日志）
    log_format json escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"ua":"$http_user_agent"'
        '}';

    access_log /data/logs/nginx-access.log json buffer=32k flush=5s;
    error_log  /data/logs/nginx-error.log warn;
}
```

### 目录关系图

```
Docker Host
├── docker-compose.yaml
├── Dockerfile                    # 只构建 sync 引擎
├── nginx.conf                    # 映射给 Nginx 容器
├── cmd/sync/                     # 你的 Go 代码
│   └── main.go
└── internal/
    └── ...

Volume: maucache                  # Docker 管理的命名卷
├── 0409MSWD2019.xml              ← sync 写入
├── 0409MSWD2019.cat              ← sync 写入
├── Microsoft_Word_16.93_Updater.pkg   ← sync 写入，Nginx 对外提供
├── collateral/
│   └── 16.93.25011212/
│       ├── 0409MSWD2019.xml
│       └── 0409MSWD2019.cat
└── ...

Volume: logs
├── nginx-access.log              ← Nginx 写入
├── nginx-error.log               ← Nginx 写入
└── sync.log                      ← sync 引擎写入
```

### 操作命令

```bash
# 一行启动全部
docker compose up -d

# 看 Nginx 访问日志（谁在下载什么）
docker compose logs -f web | jq .

# 看同步引擎日志（CDN 同步状态）
docker compose logs -f sync

# 手动触发一次同步（通过 sync 容器的管理接口）
docker compose exec sync maucache-sync --once

# 单独重启 Nginx（不影响同步）
docker compose restart web

# 单独升级 Nginx（不影响缓存数据）
docker compose pull web && docker compose up -d web

# 查看缓存占了多少空间
docker compose exec web du -sh /data/maucache
```

### 一个重要细节：原子写入

两个容器同时操作同一个 Volume，有一个要注意的事——**Nginx 不能读到写了一半的文件**。

你的 Go 同步引擎下载文件时必须这样做：

```
1. 下载到临时文件:  /data/maucache/.tmp/xxx.pkg.downloading
2. 下载完成后 rename: /data/maucache/.tmp/xxx.pkg.downloading → /data/maucache/xxx.pkg
```

`rename`（`os.Rename`）在同一个文件系统内是**原子操作**，Nginx 要么看到旧文件，要么看到新文件，不会看到半成品。这也是为什么临时目录要放在同一个 Volume 下的 `.tmp/` 子目录，而不是另一个 Volume。

Nginx 配置里加一行排除临时目录：

```nginx
# 不允许客户端访问临时目录
location /.tmp/ {
    deny all;
}
```

### 整体效果

```
现有方案:  Windows Server + IIS 配置 + PowerShell 计划任务 + 手动 MIME 配置
               ↓
容器化后:  docker compose up -d
           就这一行。两个容器自动协作，各干各的。
```

| 问题 | 现有方案怎么解决 | 容器化后怎么解决 |
|------|----------------|----------------|
| IIS 安装配置 | `PrepIISServer.ps1`（20+ 步） | `nginx:alpine` 官方镜像，零配置 |
| MIME 类型 | 手动注册 .pkg/.cat/.mpkg | `nginx.conf` 里 3 行 |
| 定时任务 | `CreateScheduledTask.ps1` | Go 内建 ticker |
| 访问日志 | 登录服务器看 IIS 日志 | `docker compose logs web` |
| 升级 Nginx | 操作系统层面升级 | `docker compose pull web` |
| 迁移到新服务器 | 重新跑一遍所有配置脚本 | 复制 `docker-compose.yaml` + `nginx.conf` |

## @kukisama

那两个容器方案，是不是对nginx的配置有一定要求？nginx的容器或者实际单机部署，有没有配置要求

## Copilot



对，Nginx 这边确实有配置要求，但好消息是**要求很少，也很明确**。让我把所有要点列清楚。

## Nginx 配置要求清单

### 核心问题就三个

```
1. root 指向哪个目录？  → /data/maucache（Volume 挂载点）
2. 哪些文件类型要能下载？ → .pkg .cat .xml .mpkg
3. 日志怎么输出？       → JSON 格式，方便查询
```

其他全用 Nginx 默认值就行。

### 最小可用配置

```nginx name=nginx.conf
server {
    listen 80;
    server_name _;

    # ========== 唯一硬性要求：root 指向缓存目录 ==========
    root /data/maucache;

    # ========== MAU 需要的 MIME 类型 ==========
    # Nginx 自带 mime.types 已覆盖 .xml，但缺以下三个
    types {
        application/octet-stream      pkg mpkg;
        application/vnd.ms-pki.seccat cat;
    }

    # ========== 目录浏览（替代 IIS 目录浏览）==========
    autoindex on;

    # ========== 大文件优化（.pkg 动辄几百MB） ==========
    sendfile on;

    # ========== 访问日志 ==========
    log_format json escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"ua":"$http_user_agent"'
        '}';
    access_log /data/logs/access.log json;

    # ========== 禁止访问临时目录 ==========
    location /.tmp/ { deny all; }
}
```

**就这些。** 没有 PHP、没有 proxy_pass、没有 upstream，纯粹的静态文件服务。

### 逐项说明：为什么需要、不配会怎样

| 配置项 | 为什么需要 | 不配会怎样 |
|--------|-----------|-----------|
| `root /data/maucache` | 告诉 Nginx 文件在哪 | **必须配**，否则 404 |
| `types { pkg mpkg }` | MAU 客户端下载 .pkg 文件 | 不配的话 Nginx 会返回 `application/octet-stream`（实际上也能下载，但不规范） |
| `types { cat }` | .cat 是 Microsoft 签名编录 | 同上，不配也能下载 |
| `autoindex on` | 管理员浏览器打开看看有什么文件 | 不配就看不到目录列表，但 MAU 客户端**不受影响**（它直接请求具体文件名） |
| `sendfile on` | 大文件传输走内核零拷贝 | 不配也能工作，但 1GB 的 pkg 传输效率低 |
| `access_log json` | 你要看日志 | 不配就用 Nginx 默认的 combined 格式，也能用，只是不方便 `jq` 查询 |
| `location /.tmp/` | 防止下载到写了一半的文件 | 不配的话客户端有小概率请求到不完整文件 |

### 哪些东西**不需要配**

| 你可能以为要配 | 实际不需要 | 原因 |
|--------------|----------|------|
| worker_processes | 不需要 | 默认 `auto`，自动匹配 CPU 核数 |
| gzip | 不需要 | .pkg 是已压缩的二进制包，gzip 没用还浪费 CPU |
| HTTPS/TLS | 不需要 | MAU 客户端在内网走 HTTP |
| proxy_pass | 不需要 | 纯静态文件，不需要反向代理 |
| client_max_body_size | 不需要 | 没有上传，只有下载 |
| fastcgi/uwsgi | 不需要 | 没有动态语言 |
| rate_limit | 不需要 | 内网场景，没人攻击你 |

## 两种部署方式的配置差异

### 方式 1：Docker 容器部署

```yaml name=docker-compose.yaml
services:
  sync:
    build: .
    volumes:
      - maucache:/data/maucache
      - logs:/data/logs
    environment:
      - MAUCACHE_SYNC_INTERVAL=6h
      - TZ=Asia/Shanghai
    restart: unless-stopped

  web:
    image: nginx:1.27-alpine
    ports:
      - "80:80"
    volumes:
      - maucache:/data/maucache:ro       # 关键：和 sync 共享同一个 volume
      - logs:/data/logs
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    restart: unless-stopped

volumes:
  maucache:
  logs:
```

配置要求：
- `nginx.conf` 文件放在 `docker-compose.yaml` 同级目录
- 映射进容器的 `/etc/nginx/conf.d/default.conf`
- **不需要动 Nginx 的主配置** `/etc/nginx/nginx.conf`，默认的就行

启动：

```bash
# 就这一行
docker compose up -d

# 验证 Nginx 配置语法是否正确
docker compose exec web nginx -t
```

### 方式 2：单机直接部署（不用 Docker）

如果你不想用 Docker，直接在 Linux 机器上跑也完全可以：

```bash
# 安装 Nginx（以 Ubuntu/Debian 为例）
apt install nginx

# 创建缓存目录
mkdir -p /data/maucache /data/logs

# 放置配置
cp nginx.conf /etc/nginx/conf.d/maucache.conf

# 删除默认站点（避免冲突）
rm /etc/nginx/sites-enabled/default

# 测试 + 重载
nginx -t && systemctl reload nginx
```

单机部署时 `nginx.conf` 唯一的区别——`root` 路径改成你的实际目录：

```nginx name=nginx-standalone.conf
server {
    listen 80;
    server_name _;

    # 单机部署：改成你 Go 同步引擎的输出目录
    root /data/maucache;

    types {
        application/octet-stream      pkg mpkg;
        application/vnd.ms-pki.seccat cat;
    }

    autoindex on;
    sendfile on;

    log_format json escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"ua":"$http_user_agent"'
        '}';
    access_log /data/logs/access.log json;

    location /.tmp/ { deny all; }
}
```

然后 Go 同步引擎也直接跑在同一台机器上，写入同一个 `/data/maucache` 目录就行。

### 两种方式对比

| | Docker 容器 | 单机直接部署 |
|---|-----------|------------|
| Nginx 来源 | `nginx:alpine` 官方镜像 | `apt install nginx` |
| 配置文件位置 | 映射到 `/etc/nginx/conf.d/` | 直接放 `/etc/nginx/conf.d/` |
| 缓存目录 | Docker named volume | 普通目录 `/data/maucache` |
| 权限处理 | Docker 自动处理 | 需确保 nginx 用户能读目录 |
| 升级 | `docker compose pull web` | `apt upgrade nginx` |
| 日志查看 | `docker compose logs web` | `tail -f /data/logs/access.log` |

## 单机部署容易踩的坑

如果选单机部署，注意以下几个点：

### 坑 1：文件权限

```bash
# Nginx 默认以 www-data 用户运行，必须能读缓存目录
chown -R www-data:www-data /data/maucache
# 或者更宽松（同步引擎可能以 root 或其他用户运行）
chmod -R 755 /data/maucache
```

Docker 方式不会有这个问题，因为容器内 Nginx 以 root 跑（alpine 镜像默认行为）。

### 坑 2：SELinux（CentOS/RHEL）

```bash
# 如果开了 SELinux，Nginx 默认不能读 /data 下的文件
semanage fcontext -a -t httpd_sys_content_t "/data/maucache(/.*)?"
restorecon -Rv /data/maucache
```

### 坑 3：端口 80 冲突

```bash
# 检查 80 端口是否被占用
ss -tlnp | grep :80

# 如果被 Apache 占了
systemctl stop apache2 && systemctl disable apache2
```

### 坑 4：日志轮转

```bash name=/etc/logrotate.d/maucache
/data/logs/access.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    postrotate
        nginx -s reopen
    endscript
}
```

Docker 方式不需要这个——用 Docker 的 logging driver 管理，或者让日志输出到 stdout。

## 总结

回到你的问题：

> Nginx 有没有配置要求？

**有，但很少。** 核心就是：

1. **`root` 指向缓存目录** — 必须
2. **补两个 MIME 类型**（`.pkg` `.cat`）— 最好配
3. **`sendfile on`** — 最好配（大文件性能）
4. **其他全用默认值**

比起现在的 `PrepIISServer.ps1` 要做的事情（安装 IIS 角色、配置站点路径、启用目录浏览、逐个添加 MIME 类型、改注册表禁用 IE ESC、重启 Explorer），Nginx 这点配置量可以忽略不计。

## @kukisama

也就是说，我可以用官方nginx的容器镜像部署，在部署之初，就可以完成所有预配置，包括证书导入么？因为我可能要换nginx证书

## Copilot



对，完全可以。Nginx 官方镜像本身就是设计成"**你只需要把配置文件和证书映射进去就行**"的模式，不需要你构建自定义镜像。

## 证书 + 配置 一起映射进去

```yaml name=docker-compose.yaml
services:
  sync:
    build: .
    volumes:
      - maucache:/data/maucache
      - logs:/data/logs
    environment:
      - MAUCACHE_SYNC_INTERVAL=6h
      - TZ=Asia/Shanghai
    restart: unless-stopped

  web:
    image: nginx:1.27-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      # 缓存文件（和 sync 共享）
      - maucache:/data/maucache:ro
      # 日志
      - logs:/data/logs
      # Nginx 配置
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
      # 证书文件
      - ./certs/server.crt:/etc/nginx/certs/server.crt:ro
      - ./certs/server.key:/etc/nginx/certs/server.key:ro
    restart: unless-stopped

volumes:
  maucache:
  logs:
```

你本地的目录结构：

```
项目目录/
├── docker-compose.yaml
├── Dockerfile                    # 同步引擎
├── nginx.conf                    # Nginx 配置
├── certs/                        # 证书目录
│   ├── server.crt                # 证书
│   └── server.key                # 私钥
└── cmd/sync/
    └── main.go
```

## Nginx 配置（带 HTTPS）

```nginx name=nginx.conf
# HTTP → 自动跳转 HTTPS
server {
    listen 80;
    server_name _;
    return 301 https://$host$request_uri;
}

# HTTPS 主站
server {
    listen 443 ssl;
    server_name _;

    # ========== 证书 ==========
    ssl_certificate     /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;

    # TLS 安全配置（内网可以宽松一些，但也别太随意）
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # ========== 文件服务 ==========
    root /data/maucache;
    autoindex on;

    types {
        application/octet-stream      pkg mpkg;
        application/vnd.ms-pki.seccat cat;
    }

    sendfile on;
    tcp_nopush on;

    # ========== 访问日志 ==========
    log_format json escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"ua":"$http_user_agent"'
        '}';
    access_log /data/logs/access.log json;

    location /.tmp/ { deny all; }
}
```

## 换证书怎么操作

### 方法 1：替换文件 + reload（不中断服务）

```bash
# 1. 把新证书放进去
cp new-cert.crt ./certs/server.crt
cp new-cert.key ./certs/server.key

# 2. 让 Nginx 重新加载（不重启容器，零中断）
docker compose exec web nginx -s reload
```

`nginx -s reload` 是热重载——正在下载 pkg 的 Mac 客户端**不会断开**，新连接用新证书。

### 方法 2：重启容器（也很快，几秒）

```bash
cp new-cert.crt ./certs/server.crt
cp new-cert.key ./certs/server.key
docker compose restart web
```

### 方法 3：用环境变量控制证书路径（灵活切换）

如果你有多套证书想切换（比如测试证书和正式证书）：

```yaml name=docker-compose.yaml
  web:
    image: nginx:1.27-alpine
    ports:
      - "443:443"
    volumes:
      - ./nginx.conf.template:/etc/nginx/templates/default.conf.template:ro
      - ./certs:/etc/nginx/certs:ro       # 整个目录映射
    environment:
      - CERT_NAME=server                   # 改这个就能切换证书
```

```nginx name=nginx.conf.template
# Nginx 官方镜像支持环境变量模板替换
ssl_certificate     /etc/nginx/certs/${CERT_NAME}.crt;
ssl_certificate_key /etc/nginx/certs/${CERT_NAME}.key;
```

```
certs/
├── server.crt          # 正式证书
├── server.key
├── test.crt            # 测试证书
└── test.key
```

```bash
# 切换到测试证书
CERT_NAME=test docker compose up -d web
```

> Nginx 官方镜像自带的 `/docker-entrypoint.d/20-envsubst-on-templates.sh` 会自动把 `/etc/nginx/templates/*.template` 里的 `${VAR}` 替换成环境变量值，然后输出到 `/etc/nginx/conf.d/`。

## 证书格式要求

Nginx 对证书格式的要求很简单：

| 文件 | 格式 | 说明 |
|------|------|------|
| `server.crt` | PEM | `-----BEGIN CERTIFICATE-----` 开头的文本文件 |
| `server.key` | PEM | `-----BEGIN PRIVATE KEY-----` 开头的文本文件 |

如果你的证书是其他格式：

```bash
# PFX/P12 → PEM（Windows 导出的证书通常是这个格式）
openssl pkcs12 -in cert.pfx -out server.crt -clcerts -nokeys
openssl pkcs12 -in cert.pfx -out server.key -nocerts -nodes

# 如果有中间证书，需要合并到 crt 文件里
cat your-cert.crt intermediate-ca.crt > server.crt
```

## 一个要注意的点：MAU 客户端和 HTTPS

MAU 客户端配置里指定缓存服务器地址的 plist：

```xml name=com.microsoft.autoupdate2.plist
<key>ManifestServer</key>
<string>https://your-maucache-server</string>
```

如果你用的是**自签名证书**或**内部 CA 签发的证书**，Mac 客户端需要信任这个 CA，否则 MAU 会拒绝连接。两种处理方式：

| 方案 | 操作 | 适合场景 |
|------|------|---------|
| **HTTP（不用证书）** | ManifestServer 写 `http://` | 内网、安全要求不高 |
| **内部 CA 证书** | 通过 MDM（如 Jamf）推送 CA 证书到所有 Mac | 企业环境 |
| **公网 CA 证书** | 买证书或用 Let's Encrypt | 有公网域名的情况 |

如果只是内网用，**HTTP 就够了**，省掉证书管理的麻烦。但你既然提到要换证书，说明你有 HTTPS 的需求，那上面的方案完全满足。

## 总结

```
你需要准备的东西：

  docker-compose.yaml     ← 复制上面的，基本不用改
  nginx.conf              ← 复制上面的，改域名（如果需要）
  certs/server.crt        ← 你的证书
  certs/server.key        ← 你的私钥

你需要执行的命令：

  docker compose up -d    ← 启动
  docker compose exec web nginx -s reload  ← 换完证书后执行

就这些。
```

## @kukisama

有些证书体系是一个脚本，可以把这种自动搞证书的脚本插进去么？另外这个容器每次更新会丢配置么？

## Copilot



两个好问题，一个一个说。

## 问题 1：自动证书脚本能不能插进去

能。常见的场景是你有一个内部 CA 或者 ACME 脚本，定期生成/续期证书。有三种方式插入：

### 方式 A：证书脚本跑在宿主机上（最简单）

```
宿主机                              Docker
┌──────────────────┐               ┌──────────────┐
│  你的证书脚本      │               │  Nginx 容器   │
│  (cron 定时跑)    │               │              │
│                  │    bind mount  │              │
│  ./certs/        │──────────────→│ /etc/nginx/  │
│   ├── server.crt │               │   certs/     │
│   └── server.key │               │              │
└──────────────────┘               └──────────────┘
```

```bash name=renew-cert.sh
#!/bin/bash
# 你现有的证书脚本，假设它会生成新证书
/opt/your-ca-tool/renew.sh \
    --domain maucache.your-company.com \
    --output /opt/maucache/certs/

# 告诉 Nginx 热加载新证书（容器不重启，连接不中断）
docker compose exec web nginx -s reload

echo "$(date) 证书已更新" >> /var/log/cert-renew.log
```

```bash
# 宿主机 crontab 里加一行，每月自动续期
0 3 1 * * /opt/maucache/renew-cert.sh
```

**为什么推荐这种**：你的证书脚本完全不需要改，跑在宿主机上就行。它生成的文件通过 bind mount 自然就进了容器。

### 方式 B：证书脚本作为第三个容器

如果你不想在宿主机上跑东西，可以把证书脚本也容器化：

```yaml name=docker-compose.yaml
services:
  sync:
    build: .
    volumes:
      - maucache:/data/maucache
    restart: unless-stopped

  web:
    image: nginx:1.27-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - maucache:/data/maucache:ro
      - certs:/etc/nginx/certs:ro          # 从共享 volume 读证书
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    restart: unless-stopped

  # 证书管理容器
  certbot:
    image: certbot/certbot                  # 以 Let's Encrypt 为例
    volumes:
      - certs:/etc/nginx/certs              # 写入证书到共享 volume
      - ./certbot-hook.sh:/opt/hook.sh:ro
    entrypoint: sh -c "certbot renew --deploy-hook /opt/hook.sh; sleep 30d"
    restart: unless-stopped

volumes:
  maucache:
  certs:       # 三个容器共享的证书目录
```

### 方式 C：用 init 脚本在 Nginx 容器启动时执行

Nginx 官方镜像有一个机制——容器启动时会自动执行 `/docker-entrypoint.d/` 下的脚本：

```yaml name=docker-compose.yaml
  web:
    image: nginx:1.27-alpine
    volumes:
      - ./init-certs.sh:/docker-entrypoint.d/30-init-certs.sh:ro
      - certs:/etc/nginx/certs
```

```bash name=init-certs.sh
#!/bin/sh
# 容器每次启动时执行
echo "正在获取/更新证书..."

# 调用你的证书工具（需要提前装好或 mount 进来）
/opt/your-ca-tool/get-cert.sh \
    --domain maucache.your-company.com \
    --cert /etc/nginx/certs/server.crt \
    --key  /etc/nginx/certs/server.key

echo "证书就绪"
```

这个脚本在 Nginx 启动**之前**执行，所以证书一定在 Nginx 读取配置之前就准备好了。

### 三种方式对比

| | 方式 A：宿主机脚本 | 方式 B：独立容器 | 方式 C：init 脚本 |
|---|---|---|---|
| 改造量 | **最小**，现有脚本不用改 | 中等 | 小 |
| 适合 | 已有证书脚本跑在服务器上 | Let's Encrypt / ACME | 内部 CA 一次性签发 |
| 续期 | 宿主机 crontab | 容器自己 sleep+loop | 容器重启时更新 |
| 依赖 | 宿主机要装证书工具 | 全容器化 | 证书工具要能装进 alpine |

## 问题 2：容器更新会丢配置吗

**不会。** 但前提是你要理解哪些东西在容器里面、哪些在外面：

```
┌──────────────────────────────��──────────────────────┐
│                    宿主机（永久）                      │
│                                                     │
│  docker-compose.yaml    ← 你的文件，永远在            │
│  nginx.conf             ← 你的文件，永远在            │
│  certs/server.crt       ← 你的文件，永远在            │
│  certs/server.key       ← 你的文件，永远在            │
│                                                     │
│  Docker Named Volume:                               │
│    maucache → /var/lib/docker/volumes/maucache/     │
│    logs     → /var/lib/docker/volumes/logs/         │
│    这些也是永久的，容器删了 volume 还在                  │
│                                                     │
│  ┌──────────────────────────────────┐               │
│  │    Nginx 容器（临时，可随时销毁重建）│               │
│  │                                  │               │
│  │  /etc/nginx/conf.d/default.conf  │ ← 从外面映射的  │
│  │  /etc/nginx/certs/server.crt     │ ← 从外面映射的  │
│  │  /data/maucache/                 │ ← volume 挂载  │
│  │  /data/logs/                     │ ← volume 挂载  │
│  │                                  │               │
│  │  容器自身的可写层：                 │               │
│  │    如果你 docker exec 进去改了什么  │               │
│  │    这些会在容器重建时丢失 ❌        │               │
│  └──────────────────────────────────┘               │
└─────────────────────────────────────────────────────┘
```

### 各种更新场景下会发生什么

| 操作 | 配置丢不丢 | 缓存文件丢不丢 | 证书丢不丢 |
|------|-----------|--------------|-----------|
| `docker compose restart web` | ❌ 不丢 | ❌ 不丢 | ❌ 不丢 |
| `docker compose down` + `up` | ❌ 不丢 | ❌ 不丢 | ❌ 不丢 |
| `docker compose pull web` + `up`（升级 Nginx） | ❌ 不丢 | ❌ 不丢 | ❌ 不丢 |
| `docker compose down -v`（⚠️ 带 `-v` 删 volume） | ❌ 不丢 | ✅ **丢！** | ❌ 不丢 |
| `docker system prune`（清理垃圾） | ❌ 不丢 | ❌ 不丢 | ❌ 不丢 |
| 宿主机重装系统 | ✅ 丢 | ✅ 丢 | ✅ 丢 |

**关键原则**：只要你的配置和证书是通过 `volumes:` 从宿主机映射进去的（bind mount），容器怎么删怎么重建都不影响。

### 唯一会丢配置的错误做法

```bash
# ❌ 错误：进容器里面直接改配置
docker compose exec web vi /etc/nginx/conf.d/default.conf
# 这样改的东西，容器重建就没了

# ✅ 正确：在宿主机上改，然后 reload
vi ./nginx.conf
docker compose exec web nginx -s reload
```

### 升级 Nginx 版本的完整流程

```bash
# 1. 改 docker-compose.yaml 里的版本号
#    nginx:1.27-alpine → nginx:1.28-alpine

# 2. 拉新镜像
docker compose pull web

# 3. 重建容器（旧容器销毁，新容器启动，配置和数据都在）
docker compose up -d web

# 整个过程 10 秒左右，配置/证书/缓存文件全部保留
```

## 完整的目录结构建议

把所有"你的东西"放在一个目录，备份这个目录就等于备份了整个系统：

```
/opt/maucache/                    # 这个目录备份了就什么都不丢
├── docker-compose.yaml           # 部署定义
├── Dockerfile                    # 同步引擎构建
├── nginx.conf                    # Nginx 配置
├── certs/                        # 证书
│   ├── server.crt
│   └── server.key
├── renew-cert.sh                 # 证书续期脚本（可选）
└── cmd/sync/                     # 你的 Go 代码
    └── main.go

# 以下由 Docker 自动管理，不用手动碰
/var/lib/docker/volumes/
├── maucache_maucache/_data/      # 缓存的 pkg/xml/cat 文件
└── maucache_logs/_data/          # 日志文件
```

迁移到新服务器：

```bash
# 老服务器
tar czf maucache-backup.tar.gz /opt/maucache/

# 新服务器
tar xzf maucache-backup.tar.gz -C /
cd /opt/maucache
docker compose up -d
# 完事。缓存文件会自动重新从 CDN 同步
```

## @kukisama

回到最初问题，你说用go写这个容易么，我们假设用两个容器关联的方案部署。还要考虑未来可能在公司内部部署的问题，就是可能不能直接从互联网去下容器镜像

## Copilot



## Go 写这个同步引擎难不难

先说结论：**不难，但也不是一天能写完的。** 核心工作量在于对接 Microsoft CDN 的 Plist XML 格式。

### 逐个模块评估

| 模块 | Go 实现难度 | 原因 |
|------|-----------|------|
| 下载 builds.txt 并解析 | ⭐ 极简 | HTTP GET + 按行 split，10 行代码 |
| Plist XML 解析 | ⭐⭐⭐ 中等 | 这是最费时的部分，需要递归解析 dict/array/string 嵌套结构 |
| 获取 20 个应用的清单 | ⭐⭐ 简单 | 并发 HTTP GET + 调用 Plist 解析器 |
| HEAD 请求获取文件元信息 | ⭐ 极简 | `http.Head()` 一行 |
| 缓存比对（文件存在+大小匹配） | ⭐ 极简 | `os.Stat()` 比较 Size |
| 并发下载 + 重试 + 原子写入 | ⭐⭐ 简单 | `errgroup` + `os.CreateTemp` + `os.Rename`，Go 天生擅长 |
| 保存 collateral 文件 | ⭐ 极简 | 和下载一样，只是文件小 |
| 定时调度 | ⭐ 极简 | `time.Ticker` |
| 配置管理（环境变量+YAML） | ⭐ 极简 | `gopkg.in/yaml.v3` 或 `envconfig` |
| healthz API | ⭐ 极简 | `http.HandleFunc` 几行 |

**真正有点工作量的就一个：Plist 解析。** 但现有 PowerShell 代码 `ConvertFrom-Plist` 已经把逻辑写得很清楚了，照着翻译就行。

### Plist 解析的实际工作量

Microsoft CDN 返回的 Plist XML 长这样：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN">
<plist version="1.0">
<array>
  <dict>
    <key>Location</key>
    <string>https://officecdnmac.microsoft.com/.../xxx.pkg</string>
    <key>Update Version</key>
    <string>16.93.25011212</string>
    <key>File Size</key>
    <integer>1048576000</integer>
  </dict>
</array>
</plist>
```

Go 的 `encoding/xml` 标准库可以直接解析，但 Plist 的 dict 结构（key-value 交替排列）不是标准 XML mapping，需要写一个自定义解析器。大概 100-150 行代码。

或者用现成的库 `howett.net/plist`，一行搞定：

```go
var packages []map[string]interface{}
plist.Unmarshal(xmlBytes, &packages)
```

### 整体代码量估计

| 文件 | 行数估计 | 作用 |
|------|---------|------|
| `main.go` | ~50 | 入口、CLI 参数 |
| `config.go` | ~60 | 配置加载 |
| `cdn.go` | ~150 | HTTP 客户端、builds.txt、HEAD 请求 |
| `plist.go` | ~120 | Plist 解析（或用库则 ~20） |
| `manifest.go` | ~100 | 获取 20 个应用的清单、组装数据 |
| `downloader.go` | ~150 | 并发下载、重试、原��写入 |
| `collateral.go` | ~80 | 保存编录文件 |
| `scheduler.go` | ~40 | 定时循环 |
| `health.go` | ~30 | /healthz 端点 |
| **合计** | **~700-800 行** | |

对比现有 PowerShell 代码（Public 7个 + Private 8个 + 主脚本 ≈ 600+ 行），Go 版本代码量差不多，但获得了并发能力和单二进制部署。

## 离线/内网部署的问题

你提到公司内部可能**不能直接拉互联网镜像**，这是企业环境的常见情况。解决方案有几层：

### 方案 1：在能上网的机器上导出镜像，拷贝到内网

这是最简单粗暴也最可靠的方式：

```bash
# ===== 能上网的机器上操作 =====

# 拉��需要的镜像
docker pull nginx:1.27-alpine
docker pull golang:1.23-alpine    # 构建用，也可以提前构建好
docker pull alpine:3.19

# 构建你的同步引擎镜像
docker compose build sync

# 导出为 tar 文件
docker save nginx:1.27-alpine -o nginx.tar
docker save maucache-sync:latest -o maucache-sync.tar

# 两个文件拷贝到 U 盘 / 内网文件服务器
# nginx.tar          ≈ 45MB
# maucache-sync.tar  ≈ 25MB
```

```bash
# ===== 内网服务器上操作 =====

# 导入镜像
docker load -i nginx.tar
docker load -i maucache-sync.tar

# 正常启动，不需要联网
docker compose up -d
```

### 方案 2：搭建内部 Harbor 镜像仓库

如果你公司有多台服务器要部署，或者需要版本管理：

```
互联网  ──→  能上网的 Harbor 代理  ──→  内网 Harbor  ──→  内网服务器
              (或手动同步)
```

```yaml name=docker-compose.yaml（内网版）
services:
  sync:
    image: harbor.your-company.com/maucache/sync:latest   # 从内部仓库拉
    volumes:
      - maucache:/data/maucache
    restart: unless-stopped

  web:
    image: harbor.your-company.com/library/nginx:1.27-alpine  # 从内部仓库拉
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - maucache:/data/maucache:ro
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
      - ./certs:/etc/nginx/certs:ro
    restart: unless-stopped

volumes:
  maucache:
```

### 方案 3：完全不用 Docker（最极端的离线场景）

如果内网连 Docker 都装不了，Go 的优势就体现出来了——**交叉编译成单二进制，扔过去就能跑**：

```bash
# 在你的开发机上编译（目标是 Linux amd64）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o maucache-sync ./cmd/sync

# 把这个二进制拷贝到内网 Linux 服务器
scp maucache-sync user@internal-server:/usr/local/bin/

# 内网服务器上直接跑，不需要 Go 环境、不需要 Docker
/usr/local/bin/maucache-sync --cache-dir /data/maucache --interval 6h
```

然后 Nginx 用系统包管理装：

```bash
# CentOS/RHEL（离线安装）
# 在能上网的机器上下载 rpm
yumdownloader nginx --resolve --destdir=./nginx-rpms/

# 拷贝到内网，本地安装
rpm -ivh ./nginx-rpms/*.rpm
```

### 三种方案对比

| | 方案 1：docker save/load | 方案 2：内部 Harbor | 方案 3：不用 Docker |
|---|---|---|---|
| 适合 | 1-3 台服务器 | 多台服务器、需要版本管理 | Docker 都不让装的环境 |
| 操作复杂度 | 低 | 中（需维护 Harbor） | 低 |
| 升级方式 | 重新导出导入 | `docker pull` 从内部仓库 | 重新编译拷贝二进制 |
| 需要拷贝的文件 | 两个 tar ≈ 70MB | 首次部署 Harbor | 一个二进制 ≈ 15MB + nginx rpm |

## 推荐的交付物结构

不管哪种方案，你最终交给内网部署的人的东西应该是一个**自包含的压缩包**：

```
maucache-release-v1.0.tar.gz
├── README.md                     # 部署说明（3步以内）
├── docker-compose.yaml           # 部署定义
├── nginx.conf                    # Nginx 配置
├── images/                       # 离线镜像（方案1）
│   ├── nginx-1.27-alpine.tar     # docker save 导出
│   └── maucache-sync-v1.0.tar    # docker save 导出
├── certs/                        # 证书（或者空目录，让用户自己放）
│   └── .gitkeep
└── install.sh                    # 一键安装脚本
```

```bash name=install.sh
#!/bin/bash
set -e

echo "=== MAUCache 安装 ==="

# 1. 导入镜像
echo "导入 Docker 镜像..."
docker load -i images/nginx-1.27-alpine.tar
docker load -i images/maucache-sync-v1.0.tar

# 2. 检查证书
if [ ! -f certs/server.crt ]; then
    echo "⚠️  请将证书放入 certs/ 目录："
    echo "    certs/server.crt"
    echo "    certs/server.key"
    echo "    然后重新运行此脚本"
    exit 1
fi

# 3. 启动
docker compose up -d
echo ""
echo "✅ 部署完成"
echo "   文件服务: https://$(hostname):443"
echo "   查看日志: docker compose logs -f"
```

部署人员拿到这个包之后：

```bash
tar xzf maucache-release-v1.0.tar.gz
cd maucache-release-v1.0

# 放入证书
cp /path/to/your/server.crt certs/
cp /path/to/your/server.key certs/

# 一键安装
bash install.sh
```

**三步完事。** 不需要理解 Docker、Nginx、Go 任何一个技术细节。

## @kukisama

我保留源码，用户自己拿去做docker是不是也很放百年？

## Copilot



对，**完全可行，而且在企业环境里可能比给他们预构建镜像更受欢迎**。

原因很现实：很多公司的安全团队不允许用外来的预构建镜像，他们要看到源码、自己构建，才放心放进生产环境。

## 源码交付的结构

```
MAUCacheAdmin.Modify/
├── README.md                          # 项目说明 + 部署指南
├── 代码架构审查.md                      # 你已有的文档
│
├── docker/                            # 容器化部署相关
│   ├── docker-compose.yaml
│   ├── nginx.conf
│   ├── Dockerfile                     # 同步引擎的构建文件
│   ├── certs/                         # 用户自己放证书
│   │   └── .gitkeep
│   └── build.sh                       # 一键构建脚本
│
├── cmd/sync/                          # Go 同步引擎入口
│   └── main.go
├── internal/                          # Go 业务逻辑
│   ├── cdn/
│   ├── sync/
│   └── config/
├── go.mod
├── go.sum
│
├── PSModule/                          # 现有 PowerShell 模块（保留）
│   └── ...
├── MacUpdatesOffice.Modify.ps1        # 现有主脚本（保留）
└── ...
```

用户拿到源码后做的事情就三步：

```bash
cd docker/

# 1. 构建（在他们自己的环境里从源码构建，安全审计友好）
docker compose build

# 2. 放证书（可选）
cp /path/to/cert.crt certs/server.crt
cp /path/to/cert.key certs/server.key

# 3. 启动
docker compose up -d
```

## 但有一个现实问题：构建时需要联网

Go 构建需要 `go mod download` 拉依赖，Dockerfile 里的 `FROM golang:1.23-alpine` 也要拉基础镜像。如果用户的构建环境也不能上网，就卡住了。

解决方案分几档：

### 档位 1：构建机能上网（最简单）

大部分公司至少有一台 CI/构建机是通网的。用户直接在上面 `docker compose build` 就行，完全不需要你操心。

### 档位 2：构建机不通网，但有内部镜像仓库

```dockerfile name=Dockerfile
# 用户改这一行，指向他们内部的镜像仓库
FROM harbor.company.com/library/golang:1.23-alpine AS builder
# ...
FROM harbor.company.com/library/alpine:3.19
```

```yaml name=docker-compose.yaml
services:
  web:
    # 用户改这一行
    image: harbor.company.com/library/nginx:1.27-alpine
```

你在 README 里说清楚：

> **内网部署提示**：如果无法访问 Docker Hub，请将 Dockerfile 和 docker-compose.yaml 中的基础镜像地址替换为贵司内部镜像仓库地址。

### 档位 3：完全断网（提供 vendor 依赖）

Go 有原生的依赖 vendor 机制：

```bash
# 你在开发时执行一次
go mod vendor

# 这会把所有依赖源码复制到 vendor/ 目录
# 然后连 vendor/ 一起提交到仓库
```

```
├── go.mod
├── go.sum
├── vendor/                     # 所有依赖的源码，离线可构建
│   ├── howett.net/plist/
│   ├── gopkg.in/yaml.v3/
│   └── modules.txt
```

Dockerfile 里改一行：

```dockerfile name=Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
# 加 -mod=vendor，不联网拉依赖，直接用 vendor/ 目录
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o /maucache-sync ./cmd/sync

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /maucache-sync /usr/local/bin/maucache-sync
VOLUME ["/data/maucache"]
ENTRYPOINT ["maucache-sync"]
```

这样用户拿到源码后，**只要有 `golang` 和 `alpine` 两个基础镜像**（这两个镜像企业内部仓库一般都有），就能在完全断网的环境下构建。

### 档位 4：极端断网（连 Docker 都用不了）

Go 交叉编译，直接给二进制：

```bash
# 你在 build.sh 里提供
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -o dist/maucache-sync-linux-amd64 ./cmd/sync
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -mod=vendor -o dist/maucache-sync-linux-arm64 ./cmd/sync
```

用户拿到二进制，配合系统自带的 Nginx，完全不需要 Docker。

## 建议的 build.sh

```bash name=docker/build.sh
#!/bin/bash
set -e

echo "=== MAUCache 构建 ==="
echo ""

# 检测是否能上网
if curl -s --connect-timeout 3 https://registry-1.docker.io/v2/ > /dev/null 2>&1; then
    echo "✅ 网络可用，直接构建"
    docker compose build
else
    echo "⚠️  无法访问 Docker Hub"
    echo ""
    echo "请选择："
    echo "  1) 修改 Dockerfile 中的基础镜像为内部仓库地址后重试"
    echo "  2) 在能上网的机器上构建，然后 docker save 导出镜像"
    echo ""
    echo "详见 README.md 中的「离线部署」章节"
    exit 1
fi

echo ""
echo "✅ 构建完成"
echo "   下一步：docker compose up -d"
```

## README 里的部署章节怎么写

````markdown name=README.md（部署章节节选）
## 部署方式

### 方式 A：有网络环境（推荐）

```bash
cd docker/
docker compose build
docker compose up -d
```

### 方式 B：离线部署（内部镜像仓库）

1. 将 `Dockerfile` 第 1、9 行的基础镜像改为内部仓库地址：
   ```
   FROM harbor.yourcompany.com/library/golang:1.23-alpine AS builder
   FROM harbor.yourcompany.com/library/alpine:3.19
   ```
2. 将 `docker-compose.yaml` 中 nginx 镜像改为：
   ```
   image: harbor.yourcompany.com/library/nginx:1.27-alpine
   ```
3. 构建并启动：
   ```bash
   docker compose build
   docker compose up -d
   ```

### 方式 C：离线部署（镜像导出导入）

在能上网的机器上：
```bash
docker compose build
docker save maucache-sync:latest -o maucache-sync.tar
docker save nginx:1.27-alpine -o nginx.tar
```

将两个 tar 文件拷贝到目标服务器：
```bash
docker load -i maucache-sync.tar
docker load -i nginx.tar
docker compose up -d
```

### 方式 D：不用 Docker

```bash
# 编译（需要 Go 1.23+）
cd cmd/sync && go build -mod=vendor -o maucache-sync .

# 运行
./maucache-sync --cache-dir /data/maucache --interval 6h

# Nginx 用系统包管理安装，配置文件见 docker/nginx.conf
```
````

## 总结

| 交付方式 | 优点 | 缺点 |
|---------|------|------|
| **给预构建镜像 tar** | 用户零学习成本 | 安全团队可能不接受；你每次发版要导出 |
| **给源码，用户自己构建** | 安全审计友好；用户自主可控 | 用户需要基础镜像（golang + alpine + nginx） |
| **源码 + vendor + 多档位文档** | **覆盖所有场景** | 你需要维护 vendor 目录和多种部署文档 |

**推荐最后一种**：源码里带上 `vendor/` 目录，README 里写清楚 A/B/C/D 四种部署方式。用户根据自己的网络条件选一种就行。这也是很多企业级 Go 项目的标准做法。

## @kukisama

根据我们的讨论，你设计一个用go来翻译现有功能的 容器方案，可以精简行为，但是保证我们的基础行为需求都满足

## Copilot

好了，我把所有源码都看完了。现在基于对每一行 PowerShell 代码的理解，给你设计一个完整的 Go 容器化方案。

---

## Go 容器化方案完整设计

### 基础需求确认

先把现有 PowerShell 做的事情提炼干净，**一件不落**：

| 步骤 | 现有代码做了什么 | Go 版必须做 |
|------|----------------|------------|
| 1 | 清理旧的 xml/cat/废弃pkg | ✅ 但要修复递归删除 collateral 的 bug |
| 2 | GET `builds.txt` → 解析为版本号数组 | ✅ |
| 3 | 对 20 个应用，GET `{AppID}.xml` / `{AppID}-chk.xml` / `{AppID}-history.xml`（可选），解析 Plist | ✅ |
| 4 | 保存编录文件到根目录（isProd）和 `collateral/{version}/` | ✅ |
| 5 | 收集所有 pkg URI，用 `builds.txt` 过滤 delta 包 | ✅ |
| 6 | 对每个 URI 发 HEAD 请求获取 size + lastModified | ✅ |
| 7 | ��对本地缓存（文件存在 + 大小匹配），不一致则下载 | ✅ |
| 8 | 下载到 scratch 目录 → move 到 cache 目录 | ✅ |
| 9 | 定时重复上述流程 | ✅ 内建调度 |

### 项目结构

```
maucache/
├── cmd/
│   └── maucache/
│       └── main.go              # 入口：CLI 解析 + 启动
│
├── internal/
│   ├── config/
│   │   └── config.go            # 配置：环境变量 + YAML
│   │
│   ├── cdn/
│   │   ├── client.go            # 复用的 HTTP 客户端（修复 P5 问题）
│   │   ├── builds.go            # 步骤2：获取 builds.txt
│   │   ├── apps.go              # 步骤3：20 个应用定义 + 获取清单
│   │   └── plist.go             # Plist XML 解析
│   │
│   ├── sync/
│   │   ├── sync.go              # 主流程编排（替代 MacUpdatesOffice.Modify.ps1）
│   │   ├── collateral.go        # 步骤4：保存编录文件
│   │   ├── planner.go           # 步骤5-6：生成下载计划
│   │   ├── downloader.go        # 步骤7-8：并发下载 + 原子写入
│   │   └── cleanup.go           # 步骤1：清理旧文件（修复 P4 bug）
│   │
│   ├── health/
│   │   └── health.go            # /healthz + /sync/status
│   │
│   └── logging/
│       └── logging.go           # 结构化 JSON 日志
│
├── go.mod
├── go.sum
├── vendor/                       # go mod vendor（离线构建用）
│
├── docker/
│   ├── Dockerfile
│   ├── docker-compose.yaml
│   └── nginx.conf
│
└── README.md
```

### 每个文件的详细设计

---

#### `cmd/maucache/main.go` — 入口

对应现有的 `MacUpdatesOffice.Modify.ps1` 的参数部分。

```go name=cmd/maucache/main.go
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"maucache/internal/config"
	"maucache/internal/health"
	"maucache/internal/logging"
	"maucache/internal/sync"
)

func main() {
	// CLI 参数（对应 PowerShell 的 param 块）
	once := flag.Bool("once", false, "执行一次同步后退出（不启动定时循环）")
	cfgPath := flag.String("config", "", "配置文件路径（可选，默认读环境变量）")
	flag.Parse()

	// 加载配置
	cfg := config.Load(*cfgPath)

	// 初始化日志
	log := logging.New(cfg.Logging.Level, cfg.Logging.Format)

	// 优雅退出
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 启动 health API（后台 goroutine）
	statusTracker := health.NewTracker()
	go health.Serve(ctx, cfg.Health.Listen, statusTracker, log)

	// 创建同步引擎
	engine := sync.NewEngine(cfg, log, statusTracker)

	if *once {
		// 单次模式：跑一次就退出
		if err := engine.RunOnce(ctx); err != nil {
			log.Error("同步失败", "error", err)
			os.Exit(1)
		}
		return
	}

	// 定时循环模式
	engine.RunLoop(ctx)
}
```

---

#### `internal/config/config.go` — 配置

对应现有的三个硬编码参数 `$workPath` / `$maupath` / `$mautemppath`，扩展为完整配置。

```go name=internal/config/config.go
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Sync    SyncConfig    `yaml:"sync"`
	Storage StorageConfig `yaml:"storage"`
	Logging LogConfig     `yaml:"logging"`
	Health  HealthConfig  `yaml:"health"`
}

type SyncConfig struct {
	Channel     string        `yaml:"channel"`     // Production / Preview / Beta
	Interval    time.Duration `yaml:"interval"`    // 同步间隔，默认 6h
	Concurrency int           `yaml:"concurrency"` // 并发下载数，默认 4
	RetryMax    int           `yaml:"retry_max"`   // 重试次数，默认 3
	RetryDelay  time.Duration `yaml:"retry_delay"` // 重试退避基数，默认 5s
}

type StorageConfig struct {
	CacheDir   string `yaml:"cache_dir"`   // 对应 $maupath → /data/maucache
	ScratchDir string `yaml:"scratch_dir"` // 对应 $mautemppath → /data/maucache/.tmp
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug / info / warn / error
	Format string `yaml:"format"` // json / text
}

type HealthConfig struct {
	Listen string `yaml:"listen"` // 管理 API 监听地址，默认 :8080
}

func Load(path string) *Config {
	cfg := &Config{
		Sync: SyncConfig{
			Channel:     envOr("MAUCACHE_SYNC_CHANNEL", "Production"),
			Interval:    durationOr("MAUCACHE_SYNC_INTERVAL", 6*time.Hour),
			Concurrency: intOr("MAUCACHE_SYNC_CONCURRENCY", 4),
			RetryMax:    intOr("MAUCACHE_SYNC_RETRY_MAX", 3),
			RetryDelay:  durationOr("MAUCACHE_SYNC_RETRY_DELAY", 5*time.Second),
		},
		Storage: StorageConfig{
			CacheDir:   envOr("MAUCACHE_CACHE_DIR", "/data/maucache"),
			ScratchDir: envOr("MAUCACHE_SCRATCH_DIR", "/data/maucache/.tmp"),
		},
		Logging: LogConfig{
			Level:  envOr("MAUCACHE_LOG_LEVEL", "info"),
			Format: envOr("MAUCACHE_LOG_FORMAT", "json"),
		},
		Health: HealthConfig{
			Listen: envOr("MAUCACHE_HEALTH_LISTEN", ":8080"),
		},
	}

	// YAML 文件如果存在则覆盖环境变量的值
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			yaml.Unmarshal(data, cfg)
		}
	}

	return cfg
}

// envOr / durationOr / intOr 是读取环境变量的辅助函数
// 省略实现，逻辑很简单
```

---

#### `internal/cdn/client.go` — HTTP 客户端

**修复现有 P5 问题**：PowerShell 每次调用都 `new HttpClient` → `Dispose`，导致端口耗尽风险。Go 版用单例。

```go name=internal/cdn/client.go
package cdn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client 封装了对 Microsoft CDN 的所有 HTTP 操作
// 全局复用一个实例，不像 PowerShell 那样每次创建新 HttpClient
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 30 * time.Minute, // 大文件下载需要足够长的超时
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// GetString 获取文本内容（用于 builds.txt 和 Plist XML）
// 对应 PowerShell: $httpClient.GetStringAsync($URI).GetAwaiter().GetResult()
func (c *Client) GetString(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// GetStringOptional 获取可选资源，404/400 返回 ""，不报错
// 对应 PowerShell: Get-PlistObjectFromURI -Optional
// CDN 对不存在的 history.xml 有时返回 400 而不是 404
func (c *Client) GetStringOptional(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil // 网络错误也当作不存在
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 400 {
		return "", nil
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// Head 发送 HEAD 请求获取文件元信息
// 对应 PowerShell: $httpClient.SendAsync($headRequest)
func (c *Client) Head(ctx context.Context, url string) (size int64, lastMod time.Time, err error) {
	req, _ := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	return resp.ContentLength, lastModTime(resp), nil
}

// Download 流式下载文件到 io.Writer
// 对应 PowerShell: Invoke-HttpClientDownload 的核心循环
func (c *Client) Download(ctx context.Context, url string, w io.Writer) (lastMod time.Time, err error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return time.Time{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// 256KB 缓冲区，与 PowerShell 一致
	buf := make([]byte, 256*1024)
	_, err = io.CopyBuffer(w, resp.Body, buf)

	return lastModTime(resp), err
}

func lastModTime(resp *http.Response) time.Time {
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		return t
	}
	return time.Time{}
}
```

---

#### `internal/cdn/builds.go` — 获取构建版本

对应 `Get-MAUProductionBuilds.ps1`（20 行 → 15 行）。

```go name=internal/cdn/builds.go
package cdn

import (
	"context"
	"strings"
)

// 频道 GUID 映射
// 对应 PowerShell Get-MAUApps 里的 switch ($Channel) 块
var channelPaths = map[string]string{
	"Production": "/pr/C1297A47-86C4-4C1F-97FA-950631F94777/MacAutoupdate/",
	"Preview":    "/pr/1ac37578-5a24-40fb-892e-b89d85b6dfaa/MacAutoupdate/",
	"Beta":       "/pr/4B2D7701-0A4F-49C8-B4CB-0C2D4043F51F/MacAutoupdate/",
}

const cdnBase = "https://officecdnmac.microsoft.com"

// FetchBuilds 获取 builds.txt 并解析为版本号切片
// 对应 PowerShell: Get-MAUProductionBuilds
func (c *Client) FetchBuilds(ctx context.Context) ([]string, error) {
	url := cdnBase + channelPaths["Production"] + "builds.txt"
	body, err := c.GetString(ctx, url)
	if err != nil {
		return nil, err
	}

	// 按行分割，过滤空行
	// 对应 PowerShell: .Split([System.Environment]::NewLine) + FixLineBreaks
	var builds []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			builds = append(builds, s)
		}
	}
	return builds, nil
}
```

---

#### `internal/cdn/apps.go` — 应用定义与清单获取

对应 `Get-MAUApps.ps1`（$targetApps 列表）+ `Get-MAUApp.ps1`（单个应用处理）。

```go name=internal/cdn/apps.go
package cdn

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
)

// AppDef 对应 PowerShell $targetApps 数组中的每一项
type AppDef struct {
	AppID   string
	AppName string
}

// 完整的 20 个应用列表，对应 Get-MAUApps.ps1 第 24-46 行
var TargetApps = []AppDef{
	{"0409MSau04", "MAU 4.x"},
	{"0409MSWD2019", "Word 365/2021/2019"},
	{"0409XCEL2019", "Excel 365/2021/2019"},
	{"0409PPT32019", "PowerPoint 365/2021/2019"},
	{"0409OPIM2019", "Outlook 365/2021/2019"},
	{"0409ONMC2019", "OneNote 365/2021/2019"},
	{"0409MSWD15", "Word 2016"},
	{"0409XCEL15", "Excel 2016"},
	{"0409PPT315", "PowerPoint 2016"},
	{"0409OPIM15", "Outlook 2016"},
	{"0409ONMC15", "OneNote 2016"},
	{"0409MSFB16", "Skype for Business"},
	{"0409IMCP01", "Intune Company Portal"},
	{"0409MSRD10", "Remote Desktop v10"},
	{"0409ONDR18", "OneDrive"},
	{"0409WDAV00", "Defender ATP"},
	{"0409EDGE01", "Edge"},
	{"0409TEAMS10", "Teams 1.0 classic"},
	{"0409TEAMS21", "Teams 2.1"},
	{"0409OLIC02", "Office Licensing Helper"},
}

// AppInfo 对应 PowerShell Get-MAUApp 返回的 PSCustomObject
type AppInfo struct {
	AppID   string
	AppName string
	Version string // 从 chk.xml 解析出来的版本号

	// 编录文件 URI
	CollateralURIs CollateralURIs

	// 从 AppID.xml 解析出的所有包的下载 URL
	PackageURIs []string

	// 从 AppID-history.xml 解析出的历史版本号
	HistoricVersions []string
	// 历史版本对应的包 URI（key=版本号）
	HistoricPackageURIs map[string][]string
}

// CollateralURIs 对应 PowerShell Get-MAUApp 里的 CollateralURIs 对象
type CollateralURIs struct {
	AppXML     string // {AppID}.xml
	CAT        string // {AppID}.cat
	ChkXml     string // {AppID}-chk.xml
	HistoryXML string // {AppID}-history.xml
}

// FetchAllApps 获取所有应用信息
// 对应 PowerShell: Get-MAUApps，但改为并发获取（原版是串行）
func (c *Client) FetchAllApps(ctx context.Context, channel string, log *slog.Logger) ([]AppInfo, error) {
	basePath, ok := channelPaths[channel]
	if !ok {
		return nil, fmt.Errorf("未知频道: %s", channel)
	}
	baseURL := cdnBase + basePath

	var (
		mu      sync.Mutex
		results []AppInfo
		wg      sync.WaitGroup
		errs    []error
	)

	// 并发获取每个应用的清单（PowerShell 原版是串行 ForEach-Object）
	sem := make(chan struct{}, 4) // 限制并发数
	for _, app := range TargetApps {
		wg.Add(1)
		go func(def AppDef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := c.fetchOneApp(ctx, def, baseURL, log)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", def.AppName, err))
				return
			}
			results = append(results, *info)
		}(app)
	}
	wg.Wait()

	if len(errs) > 0 {
		for _, e := range errs {
			log.Warn("获取应用信息失败", "error", e)
		}
	}

	return results, nil
}

// fetchOneApp 获取单个应用的完整信息
// 逐行对应 PowerShell: Get-MAUApp.ps1
func (c *Client) fetchOneApp(ctx context.Context, def AppDef, baseURL string, log *slog.Logger) (*AppInfo, error) {
	log.Info("处理应用", "appID", def.AppID, "appName", def.AppName)

	info := &AppInfo{
		AppID:   def.AppID,
		AppName: def.AppName,
		CollateralURIs: CollateralURIs{
			AppXML:     baseURL + def.AppID + ".xml",
			CAT:        baseURL + def.AppID + ".cat",
			ChkXml:     baseURL + def.AppID + "-chk.xml",
			HistoryXML: baseURL + def.AppID + "-history.xml",
		},
		HistoricPackageURIs: make(map[string][]string),
	}

	// 1. 获取 AppID.xml → 解析包列表
	// 对应 Get-MAUApp.ps1 第 37-43 行
	appXMLBody, err := c.GetString(ctx, info.CollateralURIs.AppXML)
	if err != nil {
		return nil, fmt.Errorf("获取 %s.xml 失败: %w", def.AppID, err)
	}
	packages, err := ParsePlistPackages(appXMLBody)
	if err != nil {
		return nil, fmt.Errorf("解析 %s.xml 失败: %w", def.AppID, err)
	}
	info.PackageURIs = packages.AllURIs()

	// 2. 获取 AppID-chk.xml → 解析版本号
	// 对应 Get-MAUApp.ps1 第 46-52 行
	chkBody, err := c.GetString(ctx, info.CollateralURIs.ChkXml)
	if err != nil {
		return nil, fmt.Errorf("获取 %s-chk.xml 失败: %w", def.AppID, err)
	}
	info.Version = ParsePlistVersion(chkBody)

	// 修复 version=99999 的情况
	// 对应 Get-MAUApp.ps1 第 55-59 行
	if info.Version == "99999" && len(packages.Versions) > 0 {
		info.Version = packages.Versions[0]
	}
	if info.Version == "99999" {
		info.Version = "Legacy"
	}

	// 3. 获取 AppID-history.xml（可选，404 正常）
	// 对应 Get-MAUApp.ps1 第 62-74 行
	histBody, err := c.GetStringOptional(ctx, info.CollateralURIs.HistoryXML)
	if err != nil {
		return nil, err
	}
	if histBody != "" {
		versions := ParsePlistStringArray(histBody)
		info.HistoricVersions = versions
		log.Debug("历史版本", "appID", def.AppID, "count", len(versions))

		// 获取每个历史版本的包列表
		for _, ver := range versions {
			histURL := baseURL + def.AppID + "_" + ver + ".xml"
			histXML, err := c.GetString(ctx, histURL)
			if err != nil {
				log.Warn("获取历史版本失败", "appID", def.AppID, "version", ver, "error", err)
				continue
			}
			histPkgs, _ := ParsePlistPackages(histXML)
			if histPkgs != nil {
				info.HistoricPackageURIs[ver] = histPkgs.AllURIs()
			}
		}
	}

	return info, nil
}

// ChannelBaseURL 返回频道的 CDN 基础 URL
func ChannelBaseURL(channel string) string {
	return cdnBase + channelPaths[channel]
}

// BuildVersionedURI 构建带版本号的编录 URI
// 对应 Save-MAUCollaterals.ps1 第 47-54 行的字符串操作
// 但使用 url.Parse 替代手动字符串切割（修复 P9 问题）
func BuildVersionedURI(originalURI, version, newExt string) string {
	u, err := url.Parse(originalURI)
	if err != nil {
		return ""
	}
	segments := strings.Split(u.Path, "/")
	lastSeg := segments[len(segments)-1]
	dotIdx := strings.LastIndex(lastSeg, ".")
	if dotIdx < 0 {
		return ""
	}
	baseName := lastSeg[:dotIdx]
	ext := newExt
	if ext == "" {
		ext = lastSeg[dotIdx:]
	}
	segments[len(segments)-1] = baseName + "_" + version + ext
	u.Path = strings.Join(segments, "/")
	return u.String()
}
```

---

#### `internal/cdn/plist.go` — Plist 解析

对应 `ConvertFrom-Plist.ps1` 的 134 行递归解析器。Go 只需要解析 MAU 用到的类型子集。

```go name=internal/cdn/plist.go
package cdn

import (
	"encoding/xml"
	"strings"
)

// Plist XML 结构定义
// MAU 的 Plist 只用到：dict、array、string、integer、true、false
// 对应 ConvertFrom-Plist.ps1 的 plistreader.processTree()

type plistValue struct {
	XMLName xml.Name
	Content string     `xml:",chardata"`
	Items   []plistValue `xml:",any"`
}

type plistDoc struct {
	Root plistValue `xml:",any"`
}

// ParsePlistPackages 解析 AppID.xml，提取包信息
// 输入是一个 Plist array of dict，每个 dict 包含 Location、Update Version 等 key
// 对应 PowerShell: ConvertFrom-Plist → ConvertFrom-AppPackageDictionary
type PackageList struct {
	Locations []string // Location 字段值
	Versions  []string // Update Version 字段值
}

func (p *PackageList) AllURIs() []string {
	// 去重
	seen := make(map[string]bool)
	var result []string
	for _, u := range p.Locations {
		if u != "" && !seen[u] {
			seen[u] = true
			result = append(result, u)
		}
	}
	return result
}

func ParsePlistPackages(xmlStr string) (*PackageList, error) {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return nil, err
	}

	result := &PackageList{}

	// root 应该是 array，里面的每个 item 是 dict
	arrayNode := findChild(doc.Root, "array")
	if arrayNode == nil {
		// 可能 root 本身就是 array
		arrayNode = &doc.Root
	}

	for _, dictNode := range arrayNode.Items {
		if dictNode.XMLName.Local != "dict" {
			continue
		}
		m := parseDictToMap(dictNode)
		// 收集所有下载 URL（Location、BinaryUpdaterLocation、FullUpdaterLocation）
		for _, key := range []string{"Location", "BinaryUpdaterLocation", "FullUpdaterLocation"} {
			if v, ok := m[key]; ok && v != "" {
				result.Locations = append(result.Locations, v)
			}
		}
		if v, ok := m["Update Version"]; ok {
			result.Versions = append(result.Versions, v)
		}
	}

	return result, nil
}

// ParsePlistVersion 从 chk.xml 中解析 Update Version
// 对应 Get-MAUApp.ps1 第 47-52 行
func ParsePlistVersion(xmlStr string) string {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return "unknown"
	}

	dictNode := findChild(doc.Root, "dict")
	if dictNode == nil {
		return "unknown"
	}

	m := parseDictToMap(*dictNode)
	if v, ok := m["Update Version"]; ok {
		return v
	}
	return "unknown"
}

// ParsePlistStringArray 从 history.xml 中解析版本号列表
// 对应 Get-MAUApp.ps1 第 63 行: [string[]]$historicAppVersions = Get-PlistObjectFromURI ...
func ParsePlistStringArray(xmlStr string) []string {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return nil
	}

	arrayNode := findChild(doc.Root, "array")
	if arrayNode == nil {
		return nil
	}

	var result []string
	for _, item := range arrayNode.Items {
		if item.XMLName.Local == "string" {
			result = append(result, strings.TrimSpace(item.Content))
		}
	}
	return result
}

// parseDictToMap 把 Plist dict 节点转成 Go map
// 对应 ConvertFrom-Plist.ps1 的 dict 分支：key-value 交替排列
func parseDictToMap(dict plistValue) map[string]string {
	m := make(map[string]string)
	items := dict.Items
	for i := 0; i < len(items)-1; i += 2 {
		if items[i].XMLName.Local == "key" {
			key := strings.TrimSpace(items[i].Content)
			val := items[i+1]
			switch val.XMLName.Local {
			case "string":
				m[key] = strings.TrimSpace(val.Content)
			case "integer":
				m[key] = strings.TrimSpace(val.Content)
			case "true":
				m[key] = "true"
			case "false":
				m[key] = "false"
			}
		}
	}
	return m
}

func findChild(v plistValue, name string) *plistValue {
	for i := range v.Items {
		if v.Items[i].XMLName.Local == name {
			return &v.Items[i]
		}
	}
	return nil
}
```

---

#### `internal/sync/sync.go` — 主流程编排

逐行对应 `MacUpdatesOffice.Modify.ps1` 的 59 行主脚本。

```go name=internal/sync/sync.go
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"maucache/internal/cdn"
	"maucache/internal/config"
	"maucache/internal/health"
)

type Engine struct {
	cfg     *config.Config
	client  *cdn.Client
	log     *slog.Logger
	tracker *health.Tracker
}

func NewEngine(cfg *config.Config, log *slog.Logger, tracker *health.Tracker) *Engine {
	return &Engine{
		cfg:     cfg,
		client:  cdn.NewClient(),
		log:     log,
		tracker: tracker,
	}
}

// RunOnce 执行一次完整同步
// 对应 MacUpdatesOffice.Modify.ps1 的完整流程
func (e *Engine) RunOnce(ctx context.Context) error {
	start := time.Now()
	e.tracker.SetRunning(true)
	defer e.tracker.SetRunning(false)

	e.log.Info("===== 开始同步 =====", "channel", e.cfg.Sync.Channel)

	// 步骤1: 清理旧文件（对应 ps1 第 15-36 行）
	// 修复 P4：不递归删除 collateral 目录下的文件
	cleanCount := Cleanup(e.cfg.Storage.CacheDir, e.log)
	e.log.Info("清理完成", "deleted", cleanCount)

	// 步骤2: 获取构建版本（对应 ps1 ��� 45 行）
	builds, err := e.client.FetchBuilds(ctx)
	if err != nil {
		return fmt.Errorf("获取 builds.txt 失败: %w", err)
	}
	e.log.Info("构建版本", "count", len(builds))

	// 步骤3: 获取所有应用信息（对应 ps1 第 48 行）
	apps, err := e.client.FetchAllApps(ctx, e.cfg.Sync.Channel, e.log)
	if err != nil {
		return fmt.Errorf("获取应用列表失败: %w", err)
	}
	e.log.Info("应用信息获取完成", "count", len(apps))

	// 步骤4: 保存编录文件（对应 ps1 第 51-52 行）
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, true, e.log)
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, false, e.log)

	// 步骤5-6: 生成下载计划（对应 ps1 第 55-56 行）
	jobs, err := PlanDownloads(ctx, e.client, apps, builds, e.cfg.Storage.CacheDir, e.log)
	if err != nil {
		return fmt.Errorf("生成下载计划失败: %w", err)
	}
	e.log.Info("下载计划", "total_jobs", len(jobs))

	// 步骤7-8: 执行下载（对应 ps1 第 57 行）
	result := ExecuteDownloads(ctx, e.client, jobs, e.cfg, e.log)

	elapsed := time.Since(start)
	e.tracker.RecordSync(result.Downloaded, result.Skipped, result.Failed, elapsed)
	e.log.Info("===== 同步完成 =====",
		"downloaded", result.Downloaded,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"duration", elapsed.Round(time.Second),
	)

	return nil
}

// RunLoop 定时循环执行同步
func (e *Engine) RunLoop(ctx context.Context) {
	// 启动时立即执行一次
	if err := e.RunOnce(ctx); err != nil {
		e.log.Error("同步失败", "error", err)
	}

	ticker := time.NewTicker(e.cfg.Sync.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.log.Info("收到退出信号，停止同步")
			return
		case <-ticker.C:
			if err := e.RunOnce(ctx); err != nil {
				e.log.Error("同步失败", "error", err)
			}
		}
	}
}
```

---

#### `internal/sync/planner.go` — 下载计划

对应 `Get-MAUCacheDownloadJobs.ps1`。

```go name=internal/sync/planner.go
package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"maucache/internal/cdn"
)

// DownloadJob 对应 PowerShell 返回的 PSCustomObject
// Get-MAUCacheDownloadJobs.ps1 第 74-80 行
type DownloadJob struct {
	AppName     string
	LocationURI string
	Payload     string // 文件名
	SizeBytes   int64
	LastMod     time.Time
	NeedDownload bool
}

// delta 包匹配模式：xxx_16.90.24121212_to_16.93.25011212_xxx.pkg
// 对应 Get-MAUCacheDownloadJobs.ps1 第 52 行
var deltaPattern = regexp.MustCompile(`([\d.]+)_to_([\d.]+)`)

// PlanDownloads 生成下载计划
// 对应 Get-MAUCacheDownloadJobs.ps1 + Invoke-MAUCacheDownload.ps1 的缓存验证部分
func PlanDownloads(ctx context.Context, client *cdn.Client, apps []cdn.AppInfo, builds []string, cacheDir string, log *slog.Logger) ([]DownloadJob, error) {
	buildSet := make(map[string]bool)
	for _, b := range builds {
		buildSet[b] = true
	}

	var allJobs []DownloadJob

	for _, app := range apps {
		// 收集当前版本的包 URI
		// 对应 Get-MAUCacheDownloadJobs.ps1 第 34 行
		uris := uniqueStrings(app.PackageURIs)

		// 用 builds.txt 过滤 delta 包
		// 对应 Get-MAUCacheDownloadJobs.ps1 第 50-57 行
		var filtered []string
		for _, u := range uris {
			matches := deltaPattern.FindStringSubmatch(u)
			if matches == nil {
				// 不是 delta 包，保留
				filtered = append(filtered, u)
			} else {
				// 是 delta 包，只保留 from_version 在 builds.txt 中的
				fromVer := matches[1]
				if buildSet[fromVer] {
					filtered = append(filtered, u)
				}
			}
		}

		// 对每个 URI 发 HEAD 请求 + 比对本地缓存
		for _, uri := range filtered {
			payload := filepath.Base(uri)

			// HEAD 请求获取元信息
			// 对应 Get-MAUCacheDownloadJobs.ps1 第 68-72 行
			size, lastMod, err := client.Head(ctx, uri)
			if err != nil {
				log.Warn("HEAD 请求失败", "uri", uri, "error", err)
				continue
			}

			job := DownloadJob{
				AppName:     app.AppName,
				LocationURI: uri,
				Payload:     payload,
				SizeBytes:   size,
				LastMod:     lastMod,
			}

			// 缓存验证：文件存在 + 大小匹配
			// 对应 Invoke-MAUCacheDownload.ps1 第 71-86 行
			localPath := filepath.Join(cacheDir, payload)
			fi, err := os.Stat(localPath)
			if err != nil || fi.Size() != size {
				job.NeedDownload = true
			}

			allJobs = append(allJobs, job)
		}

		log.Info("计划完成", "app", app.AppName, "packages", len(filtered))
	}

	return allJobs, nil
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
```

---

#### `internal/sync/downloader.go` — 并发下载

对应 `Invoke-MAUCacheDownload.ps1` + `Invoke-HttpClientDownload.ps1`。
**修复 P1（异常静默吞噬）、P8（重试逻辑缺陷）**。

```go name=internal/sync/downloader.go
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"maucache/internal/cdn"
	"maucache/internal/config"
	"golang.org/x/sync/errgroup"
)

type DownloadResult struct {
	Downloaded int
	Skipped    int
	Failed     int
}

// ExecuteDownloads 并发下载所有需要更新的文件
// 对应 Invoke-MAUCacheDownload.ps1 第 57-112 行的 foreach 循环
// 改进：串行 → 并发，静默失败 → 重试+报错
func ExecuteDownloads(ctx context.Context, client *cdn.Client, jobs []DownloadJob, cfg *config.Config, log *slog.Logger) DownloadResult {
	cacheDir := cfg.Storage.CacheDir
	scratchDir := cfg.Storage.ScratchDir

	// 确保目录存在
	os.MkdirAll(cacheDir, 0755)
	os.MkdirAll(scratchDir, 0755)

	var downloaded, skipped, failed atomic.Int64

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Sync.Concurrency) // 并发数限制

	for _, job := range jobs {
		job := job
		if !job.NeedDownload {
			skipped.Add(1)
			log.Info("缓存有效，跳过",
				"app", job.AppName,
				"file", job.Payload,
			)
			continue
		}

		g.Go(func() error {
			err := downloadOneFile(ctx, client, job, cacheDir, scratchDir, cfg.Sync.RetryMax, cfg.Sync.RetryDelay, log)
			if err != nil {
				failed.Add(1)
				// 不 return error，一个文件失败不阻塞其他下载
				log.Error("下载失败",
					"app", job.AppName,
					"file", job.Payload,
					"error", err,
				)
				return nil
			}
			downloaded.Add(1)
			return nil
		})
	}

	g.Wait()

	return DownloadResult{
		Downloaded: int(downloaded.Load()),
		Skipped:    int(skipped.Load()),
		Failed:     int(failed.Load()),
	}
}

// downloadOneFile 下载单个文件，带重试和原子写入
// 对应 Invoke-MAUCacheDownload.ps1 第 88-108 行
// 修复 P1: 不再静默吞噬异常
// 修复 P8: 重试次数可配置 + 指数退避
func downloadOneFile(ctx context.Context, client *cdn.Client, job DownloadJob, cacheDir, scratchDir string, maxRetry int, retryDelay time.Duration, log *slog.Logger) error {
	targetPath := filepath.Join(cacheDir, job.Payload)
	scratchPath := filepath.Join(scratchDir, job.Payload)

	log.Info("开始下载",
		"app", job.AppName,
		"file", job.Payload,
		"size_mb", job.SizeBytes/1024/1024,
	)

	var lastErr error
	for attempt := 0; attempt < maxRetry; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * retryDelay
			log.Warn("重试下载",
				"file", job.Payload,
				"attempt", attempt+1,
				"backoff", backoff,
			)
			time.Sleep(backoff)
		}

		lastErr = doDownload(ctx, client, job.LocationURI, scratchPath)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return fmt.Errorf("重试 %d 次后仍失败: %w", maxRetry, lastErr)
	}

	// 原子 rename：scratch → cache
	// 对应 Invoke-MAUCacheDownload.ps1 第 108 行: Move-Item
	if err := os.Rename(scratchPath, targetPath); err != nil {
		return fmt.Errorf("移动文件失败: %w", err)
	}

	// 设置 LastModified（对应 UseRemoteLastModified）
	if !job.LastMod.IsZero() {
		os.Chtimes(targetPath, job.LastMod, job.LastMod)
	}

	log.Info("下载完成", "file", job.Payload)
	return nil
}

// doDownload 执行一次下载（写到 scratch 路径）
func doDownload(ctx context.Context, client *cdn.Client, uri, scratchPath string) error {
	f, err := os.Create(scratchPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = client.Download(ctx, uri, f)
	if err != nil {
		os.Remove(scratchPath) // 删除不完整文件
		return err
	}
	return nil
}
```

---

#### `internal/sync/collateral.go` — 编录文件保存

对应 `Save-MAUCollaterals.ps1` + `Save-oldMAUCollaterals.ps1`。

```go name=internal/sync/collateral.go
package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"maucache/internal/cdn"
)

// SaveCollaterals 保存编录文件
// isProd=true: 保存到 cacheDir 根目录（对应 Save-MAUCollaterals isProd=$true）
// isProd=false: 保存到 cacheDir/collateral/{version}/（对应 Save-oldMAUCollaterals）
func SaveCollaterals(ctx context.Context, client *cdn.Client, apps []cdn.AppInfo, cacheDir string, isProd bool, log *slog.Logger) {
	for _, app := range apps {
		var targetDir string
		if isProd {
			targetDir = cacheDir
		} else {
			targetDir = filepath.Join(cacheDir, "collateral", app.Version)
		}
		os.MkdirAll(targetDir, 0755)

		// 基础编录 URI
		// 对应 Save-oldMAUCollaterals.ps1 第 30 行
		uris := []string{
			app.CollateralURIs.AppXML,
			app.CollateralURIs.CAT,
			app.CollateralURIs.ChkXml,
		}

		// isProd 时额外保存带版本号的 cat 和 xml
		// 对应 Save-MAUCollaterals.ps1 第 47-56 行
		if isProd {
			versionedCat := cdn.BuildVersionedURI(app.CollateralURIs.CAT, app.Version, "")
			versionedXml := cdn.BuildVersionedURI(app.CollateralURIs.CAT, app.Version, ".xml")
			if versionedCat != "" {
				uris = append(uris, versionedCat)
			}
			if versionedXml != "" {
				uris = append(uris, versionedXml)
			}
		}

		for _, uri := range uris {
			fileName := filepath.Base(uri)
			outPath := filepath.Join(targetDir, fileName)

			f, err := os.Create(outPath)
			if err != nil {
				log.Warn("创建文件失败", "path", outPath, "error", err)
				continue
			}

			lastMod, err := client.Download(ctx, uri, f)
			f.Close()
			if err != nil {
				log.Warn("下载编录失败", "uri", uri, "error", err)
				os.Remove(outPath)
				continue
			}

			if !lastMod.IsZero() {
				os.Chtimes(outPath, lastMod, lastMod)
			}
		}

		log.Debug("编录保存完成", "app", app.AppName, "dir", targetDir)
	}
}
```

---

#### `internal/sync/cleanup.go` — 文件清理

对应 `MacUpdatesOffice.Modify.ps1` 第 15-36 行，**修复 P4 bug**。

```go name=internal/sync/cleanup.go
package sync

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// 废弃文件列表
// 对应 MacUpdatesOffice.Modify.ps1 第 15-23 行的 $filesToDelete
var deprecatedFiles = []string{
	"Lync Installer.pkg",
	"MicrosoftTeams.pkg",
	"Teams_osx.pkg",
	"wdav-upgrade.pkg",
}

// Cleanup 清理旧文件
// 修复 P4：xml 和 cat 只删根目录，不递归进 collateral/
func Cleanup(cacheDir string, log *slog.Logger) int {
	count := 0

	// 1. 删除废弃的具名文件（递归，这些文件名是确定的）
	for _, name := range deprecatedFiles {
		matches, _ := filepath.Glob(filepath.Join(cacheDir, "**", name))
		for _, m := range matches {
			log.Debug("删除废弃文件", "path", m)
			os.Remove(m)
			count++
		}
	}

	// 2. 删除根目录（仅根目录！）下的 xml / cat / builds.txt
	// 修复 P4：不递归，不会误删 collateral/ 下的文件
	entries, _ := os.ReadDir(cacheDir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".xml" || ext == ".cat" || name == "builds.txt" {
			path := filepath.Join(cacheDir, name)
			log.Debug("删除根目录旧文件", "path", path)
			os.Remove(path)
			count++
		}
	}

	// 3. 清理 scratch 临时目录残留
	scratchDir := filepath.Join(cacheDir, ".tmp")
	os.RemoveAll(scratchDir)
	os.MkdirAll(scratchDir, 0755)

	return count
}
```

---

#### `internal/health/health.go` — 健康检查

PowerShell 版没有这个功能。新增，用于 Docker 健康检查和运维监控。

```go name=internal/health/health.go
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Tracker struct {
	mu         sync.RWMutex
	running    bool
	lastSync   time.Time
	downloaded int
	skipped    int
	failed     int
	duration   time.Duration
}

func NewTracker() *Tracker { return &Tracker{} }

func (t *Tracker) SetRunning(v bool) {
	t.mu.Lock()
	t.running = v
	t.mu.Unlock()
}

func (t *Tracker) RecordSync(downloaded, skipped, failed int, dur time.Duration) {
	t.mu.Lock()
	t.lastSync = time.Now()
	t.downloaded = downloaded
	t.skipped = skipped
	t.failed = failed
	t.duration = dur
	t.mu.Unlock()
}

func Serve(ctx context.Context, addr string, t *Tracker, log *slog.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/sync/status", func(w http.ResponseWriter, r *http.Request) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"running":    t.running,
			"last_sync":  t.lastSync,
			"downloaded": t.downloaded,
			"skipped":    t.skipped,
			"failed":     t.failed,
			"duration":   t.duration.String(),
		})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { <-ctx.Done(); srv.Shutdown(context.Background()) }()
	log.Info("Health API 启动", "addr", addr)
	srv.ListenAndServe()
}
```

---

#### `internal/logging/logging.go` — 日志

```go name=internal/logging/logging.go
package logging

import (
	"log/slog"
	"os"
)

func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
```

---

### 容器部署文件

#### Dockerfile

```dockerfile name=docker/Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o /maucache ./cmd/maucache

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /maucache /usr/local/bin/maucache
VOLUME ["/data/maucache"]
ENTRYPOINT ["maucache"]
```

#### docker-compose.yaml

```yaml name=docker/docker-compose.yaml
services:
  sync:
    build:
      context: ..
      dockerfile: docker/Dockerfile
    volumes:
      - maucache:/data/maucache
    environment:
      - MAUCACHE_SYNC_CHANNEL=Production
      - MAUCACHE_SYNC_INTERVAL=6h
      - MAUCACHE_SYNC_CONCURRENCY=4
      - MAUCACHE_LOG_LEVEL=info
      - MAUCACHE_CACHE_DIR=/data/maucache
      - MAUCACHE_SCRATCH_DIR=/data/maucache/.tmp
      - MAUCACHE_HEALTH_LISTEN=:8080
      - TZ=Asia/Shanghai
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 60s
      timeout: 5s
      retries: 3

  web:
    image: nginx:1.27-alpine
    ports:
      - "80:80"
    volumes:
      - maucache:/data/maucache:ro
      - logs:/data/logs
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    restart: unless-stopped

volumes:
  maucache:
  logs:
```

#### nginx.conf

```nginx name=docker/nginx.conf
server {
    listen 80;
    server_name _;
    root /data/maucache;
    autoindex on;

    types {
        application/octet-stream      pkg mpkg;
        application/vnd.ms-pki.seccat cat;
    }

    sendfile on;
    tcp_nopush on;

    log_format json escape=json
        '{'
            '"time":"$time_iso8601",'
            '"client":"$remote_addr",'
            '"method":"$request_method",'
            '"path":"$request_uri",'
            '"status":$status,'
            '"size":$body_bytes_sent,'
            '"duration":$request_time,'
            '"ua":"$http_user_agent"'
        '}';
    access_log /data/logs/access.log json;

    location /.tmp/ { deny all; }
}
```

---

### PowerShell → Go 对照总表

| PowerShell 函数/文件 | Go 文件 | Go 函数 | 修复的问题 |
|---------------------|---------|---------|-----------|
| `MacUpdatesOffice.Modify.ps1` | `sync/sync.go` | `Engine.RunOnce()` | P4(清理递归) |
| `Get-MAUProductionBuilds.ps1` | `cdn/builds.go` | `Client.FetchBuilds()` | P5(HttpClient复用) |
| `Get-MAUApps.ps1` | `cdn/apps.go` | `Client.FetchAllApps()` | 串行→并发 |
| `Get-MAUApp.ps1` | `cdn/apps.go` | `fetchOneApp()` | — |
| `ConvertFrom-Plist.ps1` | `cdn/plist.go` | `parseDictToMap()` | — |
| `ConvertFrom-AppPackageDictionary.ps1` | `cdn/plist.go` | 内联在 `ParsePlistPackages()` | — |
| `Get-PlistFromURI.ps1` | `cdn/client.go` | `GetString()`+`GetStringOptional()` | — |
| `Get-MAUCacheDownloadJobs.ps1` | `sync/planner.go` | `PlanDownloads()` | — |
| `Invoke-MAUCacheDownload.ps1` | `sync/downloader.go` | `ExecuteDownloads()` | P1(静默吞噬), P8(重试) |
| `Invoke-HttpClientDownload.ps1` | `cdn/client.go` | `Client.Download()` | P1, P5 |
| `Save-MAUCollaterals.ps1` | `sync/collateral.go` | `SaveCollaterals(isProd=true)` | P2(路径验证), P9(URI拼接) |
| `Save-oldMAUCollaterals.ps1` | `sync/collateral.go` | `SaveCollaterals(isProd=false)` | P2 |
| `PrepIISServer.ps1` | `nginx.conf` | **完全消除** | P7(路径不一致) |
| `CreateScheduledTask.ps1` | `sync/sync.go` | `Engine.RunLoop()` | P6(路径硬编码) |
| `ConvertFrom-BytesToString.ps1` | 不需要 | Go 日志直接输出 int64 | — |
| `Utilities.ps1 (FixLineBreaks)` | `cdn/builds.go` | `strings.ReplaceAll("\r\n","\n")` | — |
| `Get-HttpClientHandler.ps1` | `cdn/client.go` | `NewClient()` 单例 | P5 |
|

## @kukisama

能不能把这部分内容保存成md文件存储在我的项目中？
