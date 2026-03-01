// Package main 是 nanogrip 机器人的主程序入口
// 该文件负责初始化和启动整个机器人系统，包括：
// 1. 解析命令行参数
// 2. 加载配置文件
// 3. 初始化消息总线
// 4. 创建 LLM 提供商
// 5. 注册工具集
// 6. 启动 Agent 循环
// 7. 启动各种通信通道（Telegram, WhatsApp, Discord, Slack, DingTalk）
// 8. 处理入站和出站消息
package main

import (
	"bufio"         // bufio 用于缓冲读取
	"context"       // context 用于控制并发和取消操作
	"encoding/json" // json 用于解析消息 JSON
	"flag"          // flag 用于解析命令行参数
	"fmt"           // fmt 用于格式化输出
	"log"           // log 用于日志记录
	"os"            // os 用于操作系统功能
	"os/signal"     // os/signal 用于捕获系统信号
	"path/filepath" // filepath 用于处理文件路径
	"strings"       // strings 用于字符串操作
	"sync"          // sync 用于同步和 WaitGroup
	"syscall"       // syscall 用于系统调用
	"time"          // time 用于超时和时间处理

	// 内部包导入
	"github.com/Ailoc/nanogrip/internal/agent"     // Agent 核心逻辑
	"github.com/Ailoc/nanogrip/internal/bus"       // 消息总线
	"github.com/Ailoc/nanogrip/internal/channels"  // 通信通道
	"github.com/Ailoc/nanogrip/internal/config"    // 配置管理
	"github.com/Ailoc/nanogrip/internal/cron"      // 定时任务服务
	"github.com/Ailoc/nanogrip/internal/mcp"       // MCP 客户端
	"github.com/Ailoc/nanogrip/internal/providers" // LLM 提供商
	"github.com/Ailoc/nanogrip/internal/session"   // 会话管理
	"github.com/Ailoc/nanogrip/internal/tools"     // 工具集
)

// CLIFlags 命令行参数结构体
type CLIFlags struct {
	config  string // 配置文件路径
	command string // 子命令
	message string // 单条消息模式的消息内容 (-m)
	help    bool   // 显示帮助
}

func main() {
	// 解析命令行参数
	flags := parseFlags()

	// 如果是帮助命令，直接显示帮助并退出
	if flags.help {
		showHelp()
		return
	}

	// 处理子命令
	if flags.command != "" {
		handleCommand(flags.command, flags.config)
		return
	}

	// 默认启动 gateway 模式
	runGateway(flags.config)
}

// parseFlags 解析命令行参数
func parseFlags() *CLIFlags {
	config := flag.String("config", "", "配置文件路径 (默认: ~/.nanogrip/config.yaml)")
	message := flag.String("m", "", "单条消息模式: 直接发送消息给 Agent")
	help := flag.Bool("help", false, "显示帮助信息")
	flag.Parse()

	// 获取非标志参数（子命令）
	args := flag.Args()
	command := ""
	if len(args) > 0 {
		command = args[0]
	}

	// 如果没有子命令但有 -m 参数，则使用 agent 命令
	if command == "" && *message != "" {
		command = "agent"
	}

	// 检查环境变量
	if *config == "" {
		*config = os.Getenv("NANOGRIP_CONFIG")
	}

	return &CLIFlags{
		config:  *config,
		command: command,
		message: *message,
		help:    *help,
	}
}

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println("nanogrip - 超轻量级个人 AI 助手")
	fmt.Println("")
	fmt.Println("用法:")
	fmt.Println("  nanogrip [选项] [命令]")
	fmt.Println("")
	fmt.Println("命令:")
	fmt.Println("  agent [消息]  与 Agent 对话 (支持交互模式和单条消息模式)")
	fmt.Println("  gateway       启动 Web Gateway (默认)")
	fmt.Println("  status        查看服务状态")
	fmt.Println("  init          初始化工作区")
	fmt.Println("  cron          管理定时任务")
	fmt.Println("")
	fmt.Println("选项:")
	fmt.Println("  --config <路径>  指定配置文件路径")
	fmt.Println("  --help          显示帮助信息")
	fmt.Println("")
	fmt.Println("示例:")
	fmt.Println("  nanogrip agent                    # 交互模式")
	fmt.Println("  nanogrip agent -m \"你好\"          # 单条消息模式")
	fmt.Println("  nanogrip gateway                  # 启动 Gateway")
	fmt.Println("  nanogrip status                   # 查看状态")
	fmt.Println("  nanogrip --config /path/to/config.yaml agent -m \"你好\"")
}

// handleCommand 处理子命令
func handleCommand(command, configPath string) {
	// 获取消息参数
	message := flag.Lookup("m").Value.String()

	switch command {
	case "status":
		handleStatus(configPath)
	case "init":
		handleInit(configPath)
	case "cron":
		handleCron(configPath)
	case "gateway":
		runGateway(configPath)
	case "agent":
		// agent 子命令，支持交互模式和单条消息模式
		runAgent(configPath, message)
	default:
		fmt.Printf("未知命令: %s\n", command)
		fmt.Println("使用 nanogrip --help 查看帮助")
	}
}

// handleStatus 查看服务状态
func handleStatus(configPath string) {
	// 加载配置
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("无法加载配置: %v\n", err)
		return
	}

	workspace := cfg.GetWorkspacePath()
	fmt.Println("=== nanogrip 状态 ===")
	fmt.Printf("工作区: %s\n", workspace)
	fmt.Printf("模型: %s\n", cfg.Agents.Defaults.Model)
	fmt.Printf("最大令牌数: %d\n", cfg.Agents.Defaults.MaxTokens)
	fmt.Printf("温度: %.1f\n", cfg.Agents.Defaults.Temperature)

	// 检查 Gateway 端口
	fmt.Printf("Gateway 端口: %d\n", cfg.Gateway.Port)

	// 检查通道状态
	fmt.Println("\n通道状态:")
	if cfg.Channels.Telegram.Enabled {
		fmt.Println("  ✓ Telegram 已启用")
	}
	if cfg.Channels.WhatsApp.Enabled {
		fmt.Println("  ✓ WhatsApp 已启用")
	}
	if cfg.Channels.Discord.Enabled {
		fmt.Println("  ✓ Discord 已启用")
	}
}

// handleInit 初始化工作区
func handleInit(configPath string) {
	// 确定配置路径
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("获取主目录失败: %v\n", err)
			return
		}
		configPath = filepath.Join(home, ".nanogrip", "config.yaml")
	}

	// 确保配置目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("创建配置目录失败: %v\n", err)
		return
	}

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := getDefaultConfig()
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			fmt.Printf("创建默认配置失败: %v\n", err)
			return
		}
		fmt.Printf("默认配置已创建: %s\n", configPath)
		fmt.Println("请编辑配置文件，添加您的 API 密钥")
	}

	// 加载配置
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("无法加载配置: %v\n", err)
		return
	}

	workspace := cfg.GetWorkspacePath()
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Printf("创建工作区失败: %v\n", err)
		return
	}
	fmt.Printf("工作区已创建: %s\n", workspace)
}

// getDefaultConfig 返回默认配置内容
func getDefaultConfig() string {
	return `# nanogrip 配置文件
# 请根据您的需求修改以下配置

# LLM 提供商配置
# 支持: openai, anthropic, deepseek, openrouter, custom
providers:
  # OpenAI (GPT 系列)
  openai:
    apiKey: ""  # 填写您的 OpenAI API Key
    # apiBase: ""  # 可选：自定义 API 端点

  # Anthropic (Claude 系列)
  anthropic:
    apiKey: ""  # 填写您的 Anthropic API Key

  # DeepSeek
  deepseek:
    apiKey: ""  # 填写您的 DeepSeek API Key
    # apiBase: "https://api.deepseek.com/v1"  # 可选：自定义 API 端点

  # OpenRouter (聚合多种模型)
  openrouter:
    apiKey: ""  # 填写您的 OpenRouter API Key
    # apiBase: "https://openrouter.ai/api/v1"  # 可选：自定义 API 端点

  # 自定义提供商 (如智谱 AI)
  custom:
    apiKey: ""  # 填写您的 API Key
    apiBase: ""  # 填写 API 端点，如: https://open.bigmodel.cn/api/paas/v4

# Agent 配置
agents:
  defaults:
    model: "glm-4-flash"  # 默认模型
    maxTokens: 4096       # 最大输出令牌
    temperature: 0.7       # 温度参数
    maxToolIterations: 10  # 最大工具调用次数
    memoryWindow: 50      # 记忆窗口大小

# 工具配置
tools:
  # Web 搜索
  web:
    search:
      apiKey: ""  # Brave Search API Key
      maxResults: 5

  # Shell 命令
  exec:
    timeout: 60  # 命令超时时间(秒)

  # 文件系统
  filesystem:
    restrictToWorkspace: true  # 限制在工作区内

# 通信通道配置
channels:
  telegram:
    enabled: false
    botToken: ""

  discord:
    enabled: false
    botToken: ""

  slack:
    enabled: false
    botToken: ""

  whatsapp:
    enabled: false

  dingtalk:
    enabled: false
    secret: ""
    token: ""

# 心跳配置
heartbeat:
  enabled: false
  interval: 1800  # 秒(默认30分钟)

# 日志配置
logging:
  level: "info"  # debug, info, warn, error
`
}

// handleCron 管理定时任务
func handleCron(configPath string) {
	fmt.Println("定时任务管理:")
	fmt.Println("  使用 nanogrip cron list 查看任务列表")
	fmt.Println("  使用 nanogrip cron add <任务> 添加任务")
	fmt.Println("  使用 nanogrip cron remove <任务ID> 删除任务")
}

// loadConfig 加载配置文件
func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取主目录失败: %v", err)
		}
		configPath = filepath.Join(home, ".nanogrip", "config.yaml")
	}

	return config.Load(configPath)
}

// runAgent 运行 Agent 模式
// 支持两种模式：
// 1. 单消息模式：如果提供了 -m 参数，直接处理消息并退出
// 2. 交互式模式：启动交互式命令行界面
func runAgent(configPath, message string) {
	// 加载配置
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("无法加载配置: %v\n", err)
		fmt.Println("请创建配置文件或使用 nanogrip init 初始化")
		return
	}

	// 创建工作目录
	workspace := cfg.GetWorkspacePath()
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Printf("创建工作区失败: %v\n", err)
		return
	}

	// 配置 LLM 提供商
	var apiKey string
	apiBase := ""

	// 尝试使用自定义提供商
	if cfg.Providers.Custom.APIKey != "" {
		apiKey = cfg.Providers.Custom.APIKey
		apiBase = cfg.Providers.Custom.APIBase
	}

	// 如果没有自定义提供商，尝试 OpenRouter
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
		apiBase = cfg.Providers.OpenRouter.APIBase
	}

	// 尝试其他提供商
	if apiKey == "" {
		if cfg.Providers.Anthropic.APIKey != "" {
			apiKey = cfg.Providers.Anthropic.APIKey
		} else if cfg.Providers.OpenAI.APIKey != "" {
			apiKey = cfg.Providers.OpenAI.APIKey
		} else if cfg.Providers.DeepSeek.APIKey != "" {
			apiKey = cfg.Providers.DeepSeek.APIKey
			apiBase = cfg.Providers.DeepSeek.APIBase
		}
	}

	if apiKey == "" {
		fmt.Println("错误: 未配置 API 密钥")
		fmt.Println("请在配置文件中设置 API 密钥，或设置环境变量")
		return
	}

	extraHeaders := cfg.Providers.Custom.ExtraHeaders
	if extraHeaders == nil {
		extraHeaders = cfg.Providers.OpenRouter.ExtraHeaders
	}

	provider := providers.NewLiteLLMProvider(
		apiKey,
		apiBase,
		cfg.Agents.Defaults.Model,
		extraHeaders,
	)

	// 创建工具注册表
	toolRegistry := tools.NewToolRegistry()
	toolRegistry.Register(tools.NewShellTool(cfg.Tools.Exec.Timeout))
	toolRegistry.Register(tools.NewFilesystemTool(workspace, cfg.Tools.RestrictToWorkspace))

	// 创建会话管理器
	sessionManager := session.NewSessionManager(workspace)

	// 创建消息通道
	messageChan := make(chan string, 100)
	toolRegistry.Register(tools.NewMessageTool(messageChan))

	// 【关键修复】创建共享的消息总线，用于 AgentLoop 和子代理通信
	msgBus := bus.New(10)

	// 创建子代理管理器（使用共享的消息总线）
	subagentManager := agent.NewSubagentManager(
		provider,
		workspace,
		msgBus, // 使用共享的消息总线
		cfg.Agents.Defaults.Model,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.MaxToolIterations,
		toolRegistry,
	)

	// 注册子代理生成工具
	spawnTool := tools.NewSpawnTool(func(task string, label string, originChannel string, originChatID string) string {
		return subagentManager.Spawn(task, label, originChannel, originChatID)
	})
	toolRegistry.Register(spawnTool)

	// 注册定时任务工具（Cron）
	cronService := cron.NewCronService(func(job *cron.Job) {
		// CLI 模式下定时任务直接输出到控制台
		log.Printf("[Cron] %s -> %s", job.Name, job.Message)
	})
	cronTool := tools.NewCronTool(cronService)
	toolRegistry.Register(cronTool)
	log.Println("注册定时任务工具: cron")

	// 【关键修复】启动定时任务服务
	cronService.Start()
	log.Println("定时任务服务已启动")

	// ============================================
	// 启动 MCP 客户端（CLI 模式）
	// ============================================
	mcpManager := mcp.NewMCPManager()
	if len(cfg.MCPServers) > 0 {
		log.Printf("启动 %d 个 MCP 服务器...", len(cfg.MCPServers))
		// 转换配置格式
		mcpConfigs := make(map[string]mcp.MCPConfig)
		for name, serverConfig := range cfg.MCPServers {
			mcpConfigs[name] = mcp.MCPConfig{
				Command: serverConfig.Command,
				Args:    serverConfig.Args,
				Env:     serverConfig.Env,
				URL:     serverConfig.URL,
				Headers: serverConfig.Headers,
			}
		}
		if err := mcpManager.StartAll(mcpConfigs); err != nil {
			log.Printf("MCP 启动部分失败: %v", err)
		}
		// 注册 MCP 工具
		for _, tool := range mcpManager.GetTools() {
			toolRegistry.Register(tool)
			log.Printf("注册 MCP 工具: %s", tool.Name())
		}
	}

	// 创建 Agent 循环
	agentLoop := agent.NewAgentLoop(
		provider,
		toolRegistry,
		msgBus, // 使用共享的消息总线
		sessionManager,
		workspace,
		cfg.Agents.Defaults.Model,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxToolIterations,
		cfg.Agents.Defaults.MemoryWindow,
	)

	agentLoop.SetMessageChan(messageChan)

	// 【关键修复】启动 AgentLoop 后台 goroutine 来处理子代理结果
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer subagentManager.StopAll()
	defer cronService.Stop()
	defer mcpManager.StopAll()

	if err := agentLoop.Start(ctx); err != nil {
		fmt.Printf("启动 Agent 失败: %v\n", err)
		return
	}
	defer agentLoop.Stop()

	// 根据是否有消息决定运行模式
	if message != "" {
		// 单消息模式
		runSingleMessageMode(agentLoop, message)
	} else {
		// 交互式模式
		runInteractiveMode(agentLoop)
	}
}

// runSingleMessageMode 运行单消息模式
// 直接处理一条消息并输出结果，然后退出
func runSingleMessageMode(agentLoop *agent.AgentLoop, message string) {
	ctx := context.Background()

	fmt.Printf(">>> %s\n", message)

	response, err := agentLoop.ProcessDirect(ctx, message)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("\n%s\n", response)
}

// runInteractiveMode 运行交互式命令行界面
// 提供一个循环读取用户输入并处理的多行对话界面
func runInteractiveMode(agentLoop *agent.AgentLoop) {
	ctx := context.Background()

	fmt.Println("🐈 nanogrip 交互式对话模式")
	fmt.Println("输入您的消息，按 Enter 发送")
	fmt.Println("输入 /help 查看命令，/exit 退出")
	fmt.Println("按 Ctrl+C 退出程序")
	fmt.Println("---------------------------------------------------")

	// 设置信号捕获，支持优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 使用通道来协调退出
	done := make(chan struct{})

	// 在后台 goroutine 中读取用户输入
	reader := bufio.NewReader(os.Stdin)
	inputChan := make(chan string)

	go func() {
		for {
			fmt.Print("\n> ")
			input, err := reader.ReadString('\n')
			if err != nil {
				close(done)
				return
			}
			inputChan <- input
		}
	}()

	for {
		select {
		case <-sigChan:
			// 捕获到 Ctrl+C
			fmt.Println("\n再见!")
			return
		case <-done:
			// stdin 关闭
			return
		case input := <-inputChan:
			// 去除换行符
			input = strings.TrimSuffix(input, "\n")
			input = strings.TrimSuffix(input, "\r")

			// 处理空输入
			if input == "" {
				continue
			}

			// 处理退出命令
			if input == "/exit" || input == "/quit" {
				fmt.Println("再见!")
				return
			}

			// 处理帮助命令
			if input == "/help" {
				printInteractiveHelp()
				continue
			}

			// 处理新会话命令
			if input == "/new" {
				fmt.Println("新会话已创建")
				continue
			}

			// 处理消息
			response, err := agentLoop.ProcessDirect(ctx, input)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
				continue
			}

			fmt.Printf("\n%s\n", response)
		}
	}
}

// printInteractiveHelp 显示交互式模式的帮助信息
func printInteractiveHelp() {
	fmt.Println("")
	fmt.Println("可用命令:")
	fmt.Println("  /help    - 显示此帮助信息")
	fmt.Println("  /new     - 开始新会话")
	fmt.Println("  /exit    - 退出程序")
	fmt.Println("  Ctrl+C   - 强制退出程序")
	fmt.Println("")
	fmt.Println("提示: 您可以直接输入消息与我对话")
	fmt.Println("      我可以访问网络、运行命令和操作文件")
}
func runGateway(configPath string) {
	// ============================================
	// 第1步：加载配置文件
	// ============================================
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// ============================================
	// 第2步：创建工作目录
	// ============================================
	workspace := cfg.GetWorkspacePath()
	if err := os.MkdirAll(workspace, 0755); err != nil {
		log.Fatalf("创建工作区失败: %v", err)
	}

	// ============================================
	// 第3步：创建消息总线
	// ============================================
	msgBus := bus.New(100)

	// 用于跟踪所有后台 goroutine
	var wg sync.WaitGroup

	// ============================================
	// 第4步：配置 LLM 提供商
	// ============================================
	apiKey := ""
	apiBase := ""

	// 尝试使用自定义提供商（优先级最高）
	if cfg.Providers.Custom.APIKey != "" {
		apiKey = cfg.Providers.Custom.APIKey
		apiBase = cfg.Providers.Custom.APIBase
	}

	// 如果没有自定义提供商，尝试 OpenRouter
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
		apiBase = cfg.Providers.OpenRouter.APIBase
	}

	// 如果还没有，尝试其他提供商
	if apiKey == "" {
		if cfg.Providers.Anthropic.APIKey != "" {
			apiKey = cfg.Providers.Anthropic.APIKey
		} else if cfg.Providers.OpenAI.APIKey != "" {
			apiKey = cfg.Providers.OpenAI.APIKey
		} else if cfg.Providers.DeepSeek.APIKey != "" {
			apiKey = cfg.Providers.DeepSeek.APIKey
			apiBase = cfg.Providers.DeepSeek.APIBase
		}
	}

	if apiKey == "" {
		log.Println("Warning: 未配置 API 密钥")
	}

	extraHeaders := cfg.Providers.Custom.ExtraHeaders
	if extraHeaders == nil {
		extraHeaders = cfg.Providers.OpenRouter.ExtraHeaders
	}

	provider := providers.NewLiteLLMProvider(
		apiKey,
		apiBase,
		cfg.Agents.Defaults.Model,
		extraHeaders,
	)

	// ============================================
	// 第5步：创建工具注册表
	// ============================================
	toolRegistry := tools.NewToolRegistry()

	if cfg.Tools.Web.Search.APIKey != "" {
		// 获取搜索提供商，默认使用 brave
		provider := cfg.Tools.Web.Search.Provider
		if provider == "" {
			provider = "brave"
		}
		toolRegistry.Register(tools.NewWebSearchTool(
			cfg.Tools.Web.Search.APIKey,
			provider,
			cfg.Tools.Web.Search.MaxResults,
		))
		log.Printf("注册网络搜索工具: %s (provider: %s)", provider, cfg.Tools.Web.Search.Provider)
	} else {
		log.Println("警告: 未配置网络搜索 API Key，请在配置文件中设置 tools.web.search.apiKey 以启用搜索功能")
	}

	toolRegistry.Register(tools.NewShellTool(cfg.Tools.Exec.Timeout))
	toolRegistry.Register(tools.NewFilesystemTool(workspace, cfg.Tools.RestrictToWorkspace))

	// ============================================
	// 第6步：创建会话管理器
	// ============================================
	sessionManager := session.NewSessionManager(workspace)

	// ============================================
	// 第7步：创建消息工具
	// ============================================
	messageChan := make(chan string, 100)
	toolRegistry.Register(tools.NewMessageTool(messageChan))

	// 创建子代理管理器
	subagentManager := agent.NewSubagentManager(
		provider,
		workspace,
		msgBus,
		cfg.Agents.Defaults.Model,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.MaxToolIterations,
		toolRegistry,
	)

	// 注册子代理生成工具
	spawnTool := tools.NewSpawnTool(func(task string, label string, originChannel string, originChatID string) string {
		return subagentManager.Spawn(task, label, originChannel, originChatID)
	})
	toolRegistry.Register(spawnTool)

	// 注册定时任务工具（Cron）
	// 【方案4实现】支持 Agent 模式：可以触发 AI 执行复杂任务
	cronService := cron.NewCronService(func(job *cron.Job) {
		// 兼容旧版：Message 模式直接发送消息
		log.Printf("[Cron Runner] 发送消息: %s", job.Message)

		msg := bus.OutboundMessage{
			Channel: job.Channel,
			ChatID:  job.To,
			Content: job.Message,
			Metadata: map[string]interface{}{
				"from_cron": true,
			},
		}
		if err := msgBus.PublishOutbound(msg); err != nil {
			log.Printf("[Cron Runner] 发送消息失败: %v", err)
		}
	})
	cronTool := tools.NewCronTool(cronService)
	toolRegistry.Register(cronTool)
	log.Println("注册定时任务工具: cron")

	// ============================================
	// 第7步：启动 MCP 客户端
	// ============================================
	mcpManager := mcp.NewMCPManager()
	if len(cfg.MCPServers) > 0 {
		log.Printf("启动 %d 个 MCP 服务器...", len(cfg.MCPServers))
		// 转换配置格式
		mcpConfigs := make(map[string]mcp.MCPConfig)
		for name, serverConfig := range cfg.MCPServers {
			mcpConfigs[name] = mcp.MCPConfig{
				Command: serverConfig.Command,
				Args:    serverConfig.Args,
				Env:     serverConfig.Env,
				URL:     serverConfig.URL,
				Headers: serverConfig.Headers,
			}
		}
		if err := mcpManager.StartAll(mcpConfigs); err != nil {
			log.Printf("MCP 启动部分失败: %v", err)
		}
		// 注册 MCP 工具
		for _, tool := range mcpManager.GetTools() {
			toolRegistry.Register(tool)
			log.Printf("注册 MCP 工具: %s", tool.Name())
		}
	}

	// ============================================
	// 第8步：创建 Agent 循环
	// ============================================
	agentLoop := agent.NewAgentLoop(
		provider,
		toolRegistry,
		msgBus,
		sessionManager,
		workspace,
		cfg.Agents.Defaults.Model,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.Temperature,
		cfg.Agents.Defaults.MaxToolIterations,
		cfg.Agents.Defaults.MemoryWindow,
	)

	agentLoop.SetMessageChan(messageChan)

	// 【方案4】设置 Cron 服务的 Agent 执行器
	// 这样定时任务就可以触发 AI 执行复杂操作
	cronService.SetAgentExecutor(agentLoop)
	cronService.SetMessageBus(msgBus)
	log.Println("Cron 服务已配置 Agent 执行器")

	// 【关键修复】启动定时任务服务
	cronService.Start()
	log.Println("定时任务服务已启动")

	// ============================================
	// 第9步：创建通道管理器
	// ============================================
	channelManager := channels.NewManager(msgBus, cfg)

	// ============================================
	// 第10步：启动所有组件
	// ============================================
	ctx, cancel := context.WithCancel(context.Background())

	if err := agentLoop.Start(ctx); err != nil {
		log.Fatalf("启动 Agent 失败: %v", err)
	}

	if err := channelManager.StartAll(ctx); err != nil {
		log.Printf("Warning: 部分通道启动失败: %v", err)
	}

	// 启动 processOutbound goroutine 并注册到 WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processOutbound(ctx, msgBus, channelManager)
	}()

	// 启动消息工具输出桥接器
	// 将 messageTool 发送的 JSON 消息转换为 OutboundMessage 并发布到消息总线
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msgJSON := <-messageChan:
				// 解析 JSON 消息
				var msgData map[string]interface{}
				if err := json.Unmarshal([]byte(msgJSON), &msgData); err != nil {
					log.Printf("Failed to parse message JSON: %v", err)
					continue
				}

				// 提取字段
				content, _ := msgData["content"].(string)
				channel, _ := msgData["channel"].(string)
				chatID, _ := msgData["chat_id"].(string)
				media, _ := msgData["media"].(string)
				mediaType, _ := msgData["media_type"].(string)

				// 如果没有指定 channel，使用当前上下文
				if channel == "" {
					channel = "telegram"
				}

				// 构建媒体列表
				mediaList := []string{}
				if media != "" {
					mediaList = strings.Split(media, ",")
				}

				// 发布到消息总线
				outboundMsg := bus.OutboundMessage{
					Channel: channel,
					ChatID:  chatID,
					Content: content,
					Media:   mediaList,
					Metadata: map[string]interface{}{
						"media_type": mediaType,
					},
				}
				msgBus.PublishOutbound(outboundMsg)
			}
		}
	}()

	// ============================================
	// 第11步：运行 CLI
	// ============================================
	fmt.Println("🐈 nanogrip is running. Type /help for commands, /exit to quit.")
	runCLI(ctx, agentLoop, msgBus)

	// ============================================
	// 第12步：清理和关闭
	// ============================================
	log.Println("正在关闭...")

	// 1. 停止所有子代理
	subagentManager.StopAll()

	// 2. 取消上下文，通知所有 goroutine 退出
	cancel()

	// 3. 停止所有通信通道
	channelManager.StopAll()

	// 4. 停止 Agent 循环
	agentLoop.Stop()

	// 5. 等待所有 goroutine 完成（processOutbound, messageToolBridge）
	log.Println("等待所有 goroutine 完成...")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("所有 goroutine 已停止")
	case <-time.After(10 * time.Second):
		log.Println("警告：等待 goroutine 超时")
	}

	// 6. 停止定时任务服务
	cronService.Stop()

	// 7. 关闭 MCP 客户端
	mcpManager.StopAll()

	// 8. 最后关闭消息总线
	msgBus.Close()

	log.Println("nanogrip 已安全关闭")
}

// processOutbound 处理出站消息
func processOutbound(ctx context.Context, msgBus *bus.MessageBus, channelManager *channels.Manager) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := msgBus.ConsumeOutbound(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}

			log.Printf("[processOutbound] 收到消息: Channel=%s, ChatID=%s, Content=%.50s",
				msg.Channel, msg.ChatID, msg.Content)

			channel := channelManager.GetChannel(msg.Channel)
			if channel == nil {
				log.Printf("[processOutbound] ⚠ 警告：找不到通道 '%s'，消息丢弃", msg.Channel)
				continue
			}

			if err := channel.Send(msg); err != nil {
				log.Printf("[processOutbound] ❌ 发送消息失败: %v", err)
			} else {
				log.Printf("[processOutbound] ✓ 消息已发送到 %s (%s)", msg.Channel, msg.ChatID)
			}
		}
	}
}

// runCLI 运行命令行界面
// 阻塞等待 SIGINT/SIGTERM 信号（Ctrl+C）
func runCLI(ctx context.Context, agentLoop *agent.AgentLoop, msgBus *bus.MessageBus) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // 阻塞等待信号
}
