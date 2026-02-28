---
AIGC:
    ContentProducer: Minimax Agent AI
    ContentPropagator: Minimax Agent AI
    Label: AIGC
    ProduceID: "00000000000000000000000000000000"
    PropagateID: "00000000000000000000000000000000"
    ReservedCode1: 3045022100b25b487118d07d39b3539b3e4eefc7d835bdbc0aa58721d9279023cf7e0a285a0220442bd26092379fd3fee211ed71bc56a261a7d0767b65c97ee69861ddb64813a5
    ReservedCode2: 304402207d35e626b6c03d14b15bd1e8d9c0947262335ad5c8b0fe1e52a065381ddeab4e02202fca6fdbf9cce417d071d010d55964aa4ed595777d0a4ec24219bbe505094b32
---

# MCP (Model Context Protocol) 使用指南

## 概述

MCP（Model Context Protocol）是一种开放协议，允许 AI 助手连接到外部工具和服务。nanogrip 支持通过 MCP 连接到各种外部服务器，扩展其能力。

## 配置方法

在 `config.yaml` 文件中添加 `mcpServers` 配置：

### 方式一：命令行 MCP 服务器（Stdio）

适用于本地启动的 MCP 服务器：

```yaml
mcpServers:
  # 文件系统 MCP 服务器
  filesystem:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/workspace"]
    env:
      NODE_ENV: "production"

  # GitHub MCP 服务器
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "your-github-token"
```

### 方式二：HTTP MCP 服务器（SSE）

适用于已运行的 HTTP MCP 服务器：

```yaml
mcpServers:
  # HTTP MCP 服务器
  my-server:
    url: "http://localhost:3000"
    headers:
      Authorization: "Bearer your-token"

  # 带认证的 MCP 服务器
  auth-server:
    url: "https://mcp.example.com"
    headers:
      X-API-Key: "your-api-key"
```

## 常用 MCP 服务器

### 1. 文件系统服务器

```yaml
mcpServers:
  filesystem:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/your/path"]
```

### 2. GitHub 服务器

```yaml
mcpServers:
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "ghp_xxxxxxxxxxxx"
```

### 3. PostgreSQL 服务器

```yaml
mcpServers:
  postgres:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-postgres", "postgresql://user:pass@localhost:5432/db"]
```

### 4. Slack 服务器

```yaml
mcpServers:
  slack:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-slack"]
    env:
      SLACK_BOT_TOKEN: "xoxb-xxxxxxxxxxxx"
```

## 使用示例

配置完成后，启动 nanogrip 时会自动连接 MCP 服务器：

```bash
# CLI 模式
./nanogrip agent

# Gateway 模式
./nanogrip gateway
```

MCP 工具会自动注册到工具列表中，可以像使用内置工具一样使用它们。

## 注意事项

1. **命令型服务器**：需要安装 Node.js 和 npx
2. **HTTP 服务器**：确保服务器支持 SSE (Server-Sent Events)
3. **认证**：某些服务器需要通过环境变量或请求头提供认证信息
4. **网络**：HTTP 服务器需要网络可达

## 故障排除

### 命令未找到

确保已安装 Node.js：
```bash
node --version
npm --version
```

### 连接失败

检查服务器是否正常运行：
```bash
# HTTP 服务器
curl http://localhost:3000
```

### 工具未显示

查看日志中的 MCP 连接信息：
```bash
./nanogrip gateway 2>&1 | grep -i mcp
```
