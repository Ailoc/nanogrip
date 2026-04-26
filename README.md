<div align="center">

<div align="center">
  <img src="imgs/logo.jpeg" alt="nanogrip" width="400"/>
</div>

### 🤖 A Lightweight, Extensible AI Agent

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**NanoGrip** is a lightweight, high-performance AI agent framework written in Go, inspired by [HKUDS/nanobot](https://github.com/HKUDS/nanobot). It provides an efficient, stable foundation for building intelligent AI assistants.

</div>

---

## ⚠️ Important Notice

**Project Status**: Early Development Stage

This project is currently in the initial construction phase and **should not be considered production-ready**. The security and program robustness have not yet reached stable production standards. **Please use with caution**.

### Target Audience

This project is designed for:

- **AI Agent Enthusiasts & Learners**: Those who want to understand how AI agents work and explore agent architecture
- **Official LLM API Researchers**: Developers interested in OpenAI and Anthropic SDK integration patterns
- **Go Framework Developers**: Those studying how to build lightweight, extensible frameworks in Go
- **Contributors & Experimenters**: Developers who enjoy contributing to early-stage projects and shaping the development direction

**Not Recommended For**:
- Production environments requiring high stability and security
- Mission-critical applications
- Users expecting plug-and-play reliability without technical understanding

---

## ✨ Features

### 🧠 Intelligent Agent Core

- **LLM-Powered**: Built on top of Large Language Models for intelligent message processing
- **Tool Calling**: Multi-round tool invocation with iterative refinement
- **Context Management**: Progressive skill loading for optimized resource usage
- **Memory System**: Long-term memory (MEMORY.md) + conversation history tracking
- **Skills & MCP**

### 📢 Communication Channels

- [x] Telegram Bot
- [x] CLI (Command Line Interface)

**Telegram Interaction Example:**

<div align="center">
  <img src="imgs/telegram.jpg" alt="NanoGrip Telegram Bot Interaction" width="800"/>
</div>

### 🛠️ Built-in Tools

| Tool | Description |
|------|-------------|
| `web_search` | Web search using Brave/Tavily API |
| `filesystem` | File operations (read, write, list, delete) |
| `shell` | Execute shell commands (non-interactive) |
| `spawn` | Background subagent tasks |
| `cron` | Scheduled task management |
| `todo` | Multi-project todo list management |
| `message` | Send messages to communication channels |
| `save_memory` | Persist long-term memory |

---

## 🚀 Quick Start

### Prerequisites

- Go 1.23 or higher
- API key for your preferred LLM provider

### Installation

```bash
# Clone the repository
git clone https://github.com/Ailoc/nanogrip.git
cd nanogrip

# Build the binary
go build -o nanogrip ./cmd/nanogrip

# Initialize workspace
./nanogrip init
```

### Configuration

```bash
# Generate config in ~/.nanogrip/config.yaml
./nanogrip init

# Or copy the project example manually
cp config.example.yaml ~/.nanogrip/config.yaml

# Edit the config file
nano ~/.nanogrip/config.yaml
```

**Example `config.yaml`:**

```yaml
agents:
  defaults:
    workspace: "~/.nanogrip/workspace"
    model: "anthropic/claude-opus-4-5"
    maxTokens: 8192
    temperature: 0.7

providers:
  openai:
    apiKey: "your-openai-api-key"
    # apiBase: "https://ark.cn-beijing.volces.com/api/v3" # Optional OpenAI-compatible base URL
  anthropic:
    apiKey: "your-anthropic-api-key"

channels:
  telegram:
    enabled: true
    token: "your-telegram-bot-token"
```

### Running nanogrip

```bash
# help
./nanogrip --help

# Interactive mode
./nanogrip agent

# Single message mode
./nanogrip agent -m "Hello, nanogrip!"

# Check status
./nanogrip status

# Start Web Gateway
./nanogrip gateway
```

---

## 📁 Project Structure

```
nanogrip/
├── cmd/nanogrip/           # Main entry point
├── internal/
│   ├── agent/              # Agent core logic
│   ├── bus/                # Message bus
│   ├── channels/           # Communication channels
│   ├── config/             # Configuration management
│   ├── cron/               # Scheduled tasks
│   ├── heartbeat/          # Health monitoring
│   ├── mcp/                # MCP client
│   ├── providers/          # LLM providers
│   ├── session/            # Session management
│   ├── skills/             # Skill system
│   └── tools/              # Tool implementations
├── skills/                 # Built-in skills
├── imgs/                   # Images and assets
├── config.example.yaml     # Example configuration
└── README.md
```

---

## 🧩 Skills System

nanogrip features a progressive skill loading system:

- **Always-loaded skills**: Core skills injected into system prompt
- **On-demand skills**: Loaded when needed via filesystem tool

Built-in skills include:
- `tmux` - Interactive shell commands via tmux
- `agent-browser` - Browser automation
- `git` - Git operations
- `docker` - Docker container management
- `bash` - Bash scripting assistance
- `http` - HTTP requests
- `npm` - Node.js package management
- `postgres` - PostgreSQL database operations
- `github` - GitHub API integration
- `weather` - Weather information

---

## 🔮 Known Issues & Future Improvements

### Current Limitations

1. **Provider Support**
   - Only the official OpenAI and Anthropic APIs are supported
   - Model names should use `openai/<model>` or `anthropic/<model>`

2. **Telegram Channel File Support**
   - Telegram integration currently supports text and images only
   - File upload/download functionality is not yet implemented
   - **Planned**: Full file transfer support for Telegram channel

3. **Persistent Cron Jobs**
   - Scheduled tasks (cron jobs) only persist during runtime
   - Tasks are lost when the application restarts
   - **Planned**: Persist cron jobs to workspace for automatic restoration on startup

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- Inspired by [HKUDS/nanobot](https://github.com/HKUDS/nanobot)
- Built with [Go](https://golang.org/)
- Powered by the official OpenAI and Anthropic APIs
