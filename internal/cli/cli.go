package cli

import (
	"context"       // context 用于控制协程生命周期
	"fmt"           // fmt 用于格式化输出
	"os"            // os 用于操作系统功能
	"path/filepath" // filepath 用于处理文件路径

	"github.com/Ailoc/nanogrip/internal/config"    // 配置管理
	"github.com/Ailoc/nanogrip/internal/providers" // LLM 提供商
)

// Command CLI 命令接口
// 所有命令都实现此接口
type Command interface {
	// Name 返回命令名称
	Name() string

	// Description 返回命令描述
	Description() string

	// Run 执行命令
	// 参数：
	//   - args: 命令参数
	// 返回：
	//   - error: 执行错误
	Run(args []string) error
}

// CLI CLI 主程序
// 管理所有命令的注册和执行
type CLI struct {
	commands   map[string]Command // 命令注册表
	configPath string             // 配置文件路径
}

// NewCLI 创建新的 CLI 实例
func NewCLI(configPath string) *CLI {
	cli := &CLI{
		commands:   make(map[string]Command),
		configPath: configPath,
	}

	// 注册内置命令
	cli.registerDefaultCommands()

	return cli
}

// registerDefaultCommands 注册默认命令
func (c *CLI) registerDefaultCommands() {
	// 注册 onboard 命令
	c.RegisterCommand(&OnboardCommand{configPath: c.configPath})

	// 注册 agent 命令
	c.RegisterCommand(&AgentCommand{configPath: c.configPath})

	// 注册 gateway 命令
	c.RegisterCommand(&GatewayCommand{configPath: c.configPath})

	// 注册 status 命令
	c.RegisterCommand(&StatusCommand{})

	// 注册 channels 命令
	c.RegisterCommand(&ChannelsCommand{})

	// 注册 cron 命令
	c.RegisterCommand(&CronCommand{})
}

// RegisterCommand 注册命令
func (c *CLI) RegisterCommand(cmd Command) {
	c.commands[cmd.Name()] = cmd
}

// Run 执行命令
func (c *CLI) Run(args []string) error {
	// 如果没有参数，显示帮助
	if len(args) < 2 {
		c.PrintHelp()
		return nil
	}

	// 获取命令名称
	cmdName := args[1]

	// 查找命令
	cmd, ok := c.commands[cmdName]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmdName)
	}

	// 执行命令
	return cmd.Run(args[2:])
}

// PrintHelp 打印帮助信息
func (c *CLI) PrintHelp() {
	fmt.Println("nanogrip - 超轻量级个人 AI 助手")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  nanogrip <命令> [参数]")
	fmt.Println()
	fmt.Println("可用命令:")

	// 遍历所有命令
	for _, cmd := range c.commands {
		fmt.Printf("  %-15s %s\n", cmd.Name(), cmd.Description())
	}

	fmt.Println()
	fmt.Println("使用 'nanogrip <命令> --help' 查看命令详细信息")
}

// OnboardCommand onboard 命令
// 用于初始化配置和工作区
type OnboardCommand struct {
	configPath string
}

// Name 返回命令名称
func (c *OnboardCommand) Name() string {
	return "onboard"
}

// Description 返回命令描述
func (c *OnboardCommand) Description() string {
	return "初始化配置和工作区"
}

// Run 执行命令
func (c *OnboardCommand) Run(args []string) error {
	fmt.Println("正在初始化 nanogrip...")

	// 获取主目录
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取主目录失败: %w", err)
	}

	// 创建配置目录
	configDir := filepath.Join(home, ".nanogrip")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 创建工作区目录
	workspaceDir := filepath.Join(home, ".nanogrip", "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return fmt.Errorf("创建工作区失败: %w", err)
	}

	// 创建 memory 目录
	memoryDir := filepath.Join(workspaceDir, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("创建记忆目录失败: %w", err)
	}

	// 创建 sessions 目录
	sessionsDir := filepath.Join(workspaceDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("创建会话目录失败: %w", err)
	}

	// 创建 skills 目录
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("创建技能目录失败: %w", err)
	}

	// 创建 bootstrap 文件
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"system_prompt.md",
	}

	for _, filename := range bootstrapFiles {
		path := filepath.Join(workspaceDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// 文件不存在，创建空文件
			content := fmt.Sprintf("# %s\n\n", filename[:len(filename)-3])
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("创建 %s 失败: %w", filename, err)
			}
		}
	}

	// 创建 MEMORY.md
	memoryPath := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		content := "# 长期记忆\n\n这里存储重要的记忆和信息。\n"
		if err := os.WriteFile(memoryPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("创建 MEMORY.md 失败: %w", err)
		}
	}

	// 创建 HISTORY.md
	historyPath := filepath.Join(memoryDir, "HISTORY.md")
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		content := "# 对话历史\n\n这是可搜索的对话历史记录。\n"
		if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("创建 HISTORY.md 失败: %w", err)
		}
	}

	// 创建示例配置文件
	exampleConfigPath := filepath.Join(configDir, "config.example.yaml")
	if _, err := os.Stat(exampleConfigPath); os.IsNotExist(err) {
		exampleConfig := `agents:
  defaults:
    workspace: "~/.nanogrip/workspace"
    model: "glm-4-flash"
    maxTokens: 8192
    temperature: 0.7
    maxToolIterations: 20
    memoryWindow: 50

providers:
  custom:
    apiKey: ""  # 填入你的 API Key
    apiBase: ""  # 填入 API 基础 URL

channels:
  telegram:
    enabled: false
    token: ""
    allowFrom: []

gateway:
  host: "0.0.0.0"
  port: 18790

tools:
  web:
    search:
      apiKey: ""
  exec:
    timeout: 60
  restrictToWorkspace: true
`
		if err := os.WriteFile(exampleConfigPath, []byte(exampleConfig), 0644); err != nil {
			return fmt.Errorf("创建示例配置失败: %w", err)
		}
	}

	fmt.Println("✅ 初始化完成!")
	fmt.Println()
	fmt.Println("目录结构:")
	fmt.Printf("  ~/.nanogrip/\n")
	fmt.Printf("  ├── config.yaml       # 配置文件（需要手动创建）\n")
	fmt.Printf("  └── workspace/\n")
	fmt.Printf("      ├── memory/\n")
	fmt.Printf("      │   ├── MEMORY.md    # 长期记忆\n")
	fmt.Printf("      │   └── HISTORY.md   # 对话历史\n")
	fmt.Printf("      ├── skills/        # 技能目录\n")
	fmt.Printf("      └── sessions/      # 会话存储\n")
	fmt.Println()
	fmt.Println("下一步:")
	fmt.Println("  1. 复制配置文件: cp ~/.nanogrip/config.example.yaml ~/.nanogrip/config.yaml")
	fmt.Println("  2. 编辑配置文件，填入你的 API Key")
	fmt.Println("  3. 运行机器人: nanogrip gateway")

	return nil
}

// AgentCommand agent 命令
// 用于与 Agent 对话
type AgentCommand struct {
	configPath string
}

// Name 返回命令名称
func (c *AgentCommand) Name() string {
	return "agent"
}

// Description 返回命令描述
func (c *AgentCommand) Description() string {
	return "与 Agent 对话"
}

// Run 执行命令
func (c *AgentCommand) Run(args []string) error {
	// 加载配置
	cfg, err := config.Load(c.configPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 获取 API Key
	apiKey := cfg.Providers.Custom.APIKey
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
	}

	if apiKey == "" {
		return fmt.Errorf("未配置 API Key")
	}

	// 创建 Provider
	provider := providers.NewLiteLLMProvider(
		apiKey,
		cfg.Providers.Custom.APIBase,
		cfg.Agents.Defaults.Model,
		nil,
	)

	// 如果没有消息参数，进入交互模式
	if len(args) == 0 {
		return c.runInteractive(provider, cfg)
	}

	// 单次对话
	ctx := context.Background()
	resp, err := provider.Chat(
		ctx,
		[]providers.Message{
			{Role: "user", Content: args[0]},
		},
		nil,
		cfg.Agents.Defaults.Model,
		cfg.Agents.Defaults.MaxTokens,
		cfg.Agents.Defaults.Temperature,
	)

	if err != nil {
		return fmt.Errorf("调用失败: %w", err)
	}

	fmt.Println(resp.Content)
	return nil
}

// runInteractive 运行交互模式
func (c *AgentCommand) runInteractive(provider providers.LLMProvider, cfg *config.Config) error {
	fmt.Println("进入交互模式，输入 'exit' 退出")
	fmt.Println()

	ctx := context.Background()

	for {
		fmt.Print("> ")
		var input string
		fmt.Scanln(&input)

		if input == "exit" || input == "quit" {
			break
		}

		if input == "" {
			continue
		}

		resp, err := provider.Chat(
			ctx,
			[]providers.Message{
				{Role: "user", Content: input},
			},
			nil,
			cfg.Agents.Defaults.Model,
			cfg.Agents.Defaults.MaxTokens,
			cfg.Agents.Defaults.Temperature,
		)

		if err != nil {
			fmt.Printf("错误: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Println(resp.Content)
		fmt.Println()
	}

	return nil
}

// GatewayCommand gateway 命令
// 用于启动网关
type GatewayCommand struct {
	configPath string
}

// Name 返回命令名称
func (c *GatewayCommand) Name() string {
	return "gateway"
}

// Description 返回命令描述
func (c *GatewayCommand) Description() string {
	return "启动网关"
}

// Run 执行命令
func (c *GatewayCommand) Run(args []string) error {
	fmt.Println("启动网关...")
	fmt.Println("提示: 请使用 'nanogrip' 命令启动完整程序")

	// TODO: 实现完整的网关启动逻辑
	return nil
}

// StatusCommand status 命令
// 用于显示状态
type StatusCommand struct {
}

// Name 返回命令名称
func (c *StatusCommand) Name() string {
	return "status"
}

// Description 返回命令描述
func (c *StatusCommand) Description() string {
	return "显示状态"
}

// Run 执行命令
func (c *StatusCommand) Run(args []string) error {
	fmt.Println("=== nanogrip 状态 ===")
	fmt.Println()
	fmt.Println("版本: 1.0.0")
	fmt.Println("状态: 未运行")

	return nil
}

// ChannelsCommand channels 命令
// 用于管理通道
type ChannelsCommand struct {
}

// Name 返回命令名称
func (c *ChannelsCommand) Name() string {
	return "channels"
}

// Description 返回命令描述
func (c *ChannelsCommand) Description() string {
	return "管理通道"
}

// Run 执行命令
func (c *ChannelsCommand) Run(args []string) error {
	if len(args) == 0 {
		// 显示通道状态
		fmt.Println("=== 通道状态 ===")
		fmt.Println()
		fmt.Println("Telegram:  未配置")
		fmt.Println("WhatsApp:  未配置")
		fmt.Println("Discord:   未配置")
		fmt.Println("Slack:     未配置")
		fmt.Println("DingTalk:  未配置")
		return nil
	}

	// 处理子命令
	switch args[0] {
	case "login":
		fmt.Println("WhatsApp 登录:")
		fmt.Println("请运行 'nanogrip gateway' 并访问 Web 界面进行登录")
	case "status":
		fmt.Println("Telegram:  未配置")
		fmt.Println("WhatsApp:  未配置")
	default:
		return fmt.Errorf("未知子命令: %s", args[0])
	}

	return nil
}

// CronCommand cron 命令
// 用于管理定时任务
type CronCommand struct {
}

// Name 返回命令名称
func (c *CronCommand) Name() string {
	return "cron"
}

// Description 返回命令描述
func (c *CronCommand) Description() string {
	return "管理定时任务"
}

// Run 执行命令
func (c *CronCommand) Run(args []string) error {
	if len(args) == 0 {
		// 显示帮助
		fmt.Println("用法: nanogrip cron <子命令>")
		fmt.Println()
		fmt.Println("子命令:")
		fmt.Println("  add     添加定时任务")
		fmt.Println("  list    列出所有定时任务")
		fmt.Println("  remove  删除定时任务")
		return nil
	}

	// 处理子命令
	switch args[0] {
	case "list":
		fmt.Println("=== 定时任务 ===")
		fmt.Println()
		fmt.Println("（暂无定时任务）")
	case "add":
		fmt.Println("添加定时任务")
		fmt.Println("提示: 请在配置文件中添加 cron 任务")
	case "remove":
		fmt.Println("删除定时任务")
		fmt.Println("提示: 请在配置文件中删除 cron 任务")
	default:
		return fmt.Errorf("未知子命令: %s", args[0])
	}

	return nil
}
