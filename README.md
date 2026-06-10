# cpa-core

**cpa-core** 是一个 AI 代理服务，为 CLI 工具提供 OpenAI、Gemini、Claude、Codex、Grok 等兼容 API 接口。支持 OAuth 登录、多账户负载均衡、请求监控、配额管理等功能。

配套 Web 管理界面：[cpa-web](https://github.com/fxzer/cpa-web)

---

## 架构

```
cpa-web (React SPA)
    │  全部页面 → /v0/management/*
    ▼
cpa-core :8317
    AI 代理 + Management API + SQLite 请求事件
```

---

## 快速开始

### 前置依赖

- Go ≥ 1.26
- Node.js + npm（构建前端时需要）
- git

### 本地开发

```bash
# 克隆
 git clone https://github.com/fxzer/cpa-core.git
 cd cpa-core

# 配置
 cp config.example.yaml config.yaml
 # 编辑 config.yaml，设置管理密钥等

# 启动
 go run ./cmd/server -config config.yaml
```

访问 `http://localhost:8317/healthz` 确认服务运行正常。

### 部署管理界面

```bash
# 克隆前端仓库
 git clone https://github.com/fxzer/cpa-web.git
 cd cpa-web

# 构建前端
 npm install && npm run build

# 部署到 cpa-core static 目录
 cp dist/index.html /path/to/cpa-core/static/web.html

# 或使用前端的一键部署脚本
 ./deploy.sh --to /path/to/cpa-core/static
```

然后重启 cpa-core 服务，访问 `http://localhost:8317/web.html`。

---

## 部署

### 手动编译

```bash
 go build -o cpa-core ./cmd/server
 ./cpa-core -config config.yaml
```

### Docker

```bash
 docker build -t cpa-core .
 docker-compose up -d
```

### 管理界面

管理界面为单文件 HTML（web.html），由 cpa-core 托管。部署方式见上方「部署管理界面」。
也可以直接从 GitHub Releases 下载预构建的 web.html：

```bash
 curl -L -o web.html https://github.com/fxzer/cpa-web/releases/latest/download/web.html
 cp web.html /opt/cpa-core/static/web.html
```

---

## fxzer Fork：Management API 扩展

本 fork 在官方上游基础上新增了以下 Management API，用于支撑 cpa-web 的管理功能：

| API | 说明 |
|-----|------|
| `GET /v0/management/request-events` | 分页查询请求事件 |
| `GET /v0/management/request-events/status` | 持久化状态（事件数、写入队列等） |
| `GET /v0/management/request-events/export` | 导出 JSONL |
| `POST /v0/management/request-events/import` | 导入 JSONL |
| `DELETE /v0/management/request-events` | 清空事件 |
| `GET /v0/management/usage` | 聚合用量数据 |
| `GET /v0/management/auth-refresh-queue` | Auth Refresh Queue 快照 |
| `GET/PUT /v0/management/model-prices` | 模型定价 |
| `POST /v0/management/model-prices/sync-litellm` | 从 LiteLLM 同步定价 |

### 内部改动

- Redis Queue 监控：新增 `PeekAll()` 方法
- SQLite 请求事件持久化（默认开启）
- 管理页面自动更新机制（GitHub Release 检测 + 由底兜底 URL）
- 统一管理页面文件名为 `web.html`

---

## 功能

### 多 AI 提供商支持

支持 OpenAI、Gemini、Claude、Codex、Grok 等主流提供商，提供统一兼容 API。

### OAuth 登录

支持 OpenAI Codex、Claude Code、Grok Build 等 CLI 工具的 OAuth 流程。

### 多账户负载均衡

同一提供商可配置多个 API Key，自动轮询/负载均衡。

### 请求监控

请求事件持久化到 SQLite，支持分页查询、导出导入、定价计算。

### 可嵌入 Go SDK

提供 Go SDK，可将代理功能嵌入自有应用。

---

## 配置

参考 `config.example.yaml`，主要配置项：

```yaml
port: 8317
remote-management:
  secret-key: "管理密钥"
  allow-remote: true
usage-statistics-enabled: true
usage:
  enabled: true
```

---

## 技术栈

- Go 1.26
- Gin（HTTP 框架）
- SQLite（请求事件持久化）
- Redis（队列管理）
- gorilla/websocket
- charmbracelet/bubbletea（TUI）

---

## 许可证

MIT
