// context.go - 上下文构建器
//
// 这个文件包含 ContextBuilder，负责构建发送给 LLM 的完整上下文，包括：
// 1. 系统提示词（身份、时间、工作空间信息）
// 2. Bootstrap 文件（AGENTS.md、SOUL.md、USER.md 等）
// 3. 技能系统（始终加载的技能 + 可用技能摘要）
// 4. 历史消息
// 5. 当前用户消息
//
// 技能加载策略：
// - Always-loaded skills: 完整内容注入到系统提示词中
// - Available skills: 只显示摘要，Agent 需要时可以通过 read_file 工具读取
package agent

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Ailoc/nanogrip/internal/skills"
)

// ContextBuilder 构建发送给 LLM 的消息
// 它负责组装完整的上下文，包括系统提示词、历史消息和当前消息
type ContextBuilder struct {
	workspace   string               // 工作空间路径
	skills      *skills.SkillsLoader // 技能加载器
	memoryStore *MemoryStore         // 记忆存储
}

// NewContextBuilder 创建一个新的上下文构建器
// 参数：
//   - workspace: 工作空间路径
//   - builtinSkills: 内置技能路径
func NewContextBuilder(workspace string, builtinSkills string) *ContextBuilder {
	skillsLoader := skills.NewSkillsLoader(workspace, builtinSkills)
	return &ContextBuilder{
		workspace: workspace,
		skills:    skillsLoader,
	}
}

// SetMemoryStore 设置记忆存储
func (cb *ContextBuilder) SetMemoryStore(memoryStore *MemoryStore) {
	cb.memoryStore = memoryStore
}

// BuildMessages 构建完整的消息数组
// 这个方法组装所有必要的上下文，包括：
// 1. 系统消息（包含身份、技能、工作空间信息）
// 2. 历史消息（从会话中获取）
// 3. 当前用户消息
//
// 参数：
//   - history: 会话历史消息
//   - currentMessage: 当前用户消息
//   - channel: 消息来源频道（如 "whatsapp"、"cli"）
//   - chatID: 聊天 ID
//   - media: 媒体文件列表（如图片、文件）
func (cb *ContextBuilder) BuildMessages(
	history []map[string]interface{},
	currentMessage string,
	channel string,
	chatID string,
	media []string,
) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0)

	// 系统消息 - 包含 Agent 身份、技能、工作空间等核心信息
	systemContent := cb.buildSystemPrompt()
	if channel != "" {
		systemContent += fmt.Sprintf("\n\nCurrent channel: %s", channel)
	}
	if chatID != "" {
		systemContent += fmt.Sprintf("\nChat ID: %s", chatID)
	}
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": systemContent,
	})

	// 历史消息 - 保留对话上下文
	for _, msg := range history {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)

		if role == "" || content == "" {
			continue
		}

		entry := map[string]interface{}{
			"role":    role,
			"content": content,
		}

		// 添加工具调用信息（如果存在）
		if tc, ok := msg["tool_calls"].([]interface{}); ok && len(tc) > 0 {
			entry["tool_calls"] = tc
		}
		if tcID, ok := msg["tool_call_id"].(string); ok && tcID != "" {
			entry["tool_call_id"] = tcID
		}

		messages = append(messages, entry)
	}

	// 当前用户消息 - 添加媒体附件信息
	currentContent := currentMessage
	var imageList []string // 收集图片用于发送给视觉模型

	if len(media) > 0 {
		for _, m := range media {
			// 检测媒体类型：URL、base64 data URL 或本地文件
			if strings.HasPrefix(m, "http://") || strings.HasPrefix(m, "https://") {
				// URL - 添加到内容中，同时作为图片引用
				currentContent += "\n[Media URL: " + m + "]"
				// 如果是图片 URL，添加到图片列表
				if strings.HasSuffix(m, ".jpg") || strings.HasSuffix(m, ".jpeg") ||
					strings.HasSuffix(m, ".png") || strings.HasSuffix(m, ".gif") ||
					strings.HasSuffix(m, ".webp") {
					imageList = append(imageList, m)
				}
			} else if strings.HasPrefix(m, "data:image/") {
				// base64 图片 - 不添加到文本内容，添加到图片列表供视觉模型使用
				imageList = append(imageList, m)
			} else {
				currentContent += "\n[File: " + m + "]"
			}
		}
	}

	msg := map[string]interface{}{
		"role":    "user",
		"content": currentContent,
	}

	// 添加图片列表（如果有）供视觉模型使用
	if len(imageList) > 0 {
		msg["images"] = imageList
	}

	messages = append(messages, msg)

	return messages
}

// buildSystemPrompt 构建完整的系统提示词
// 系统提示词是 Agent 的核心"大脑"，包含：
// 1. 核心身份（Agent 名称、能力、运行环境）
// 2. Bootstrap 文件（AGENTS.md、SOUL.md 等自定义配置）
// 3. 技能系统（always-loaded 技能的完整内容 + 可用技能摘要）
//
// 技能加载策略（渐进式加载）：
// - Always-loaded skills: 完整内容直接注入（如核心技能）
// - Available skills: 只显示摘要，需要时 Agent 通过 read_file 工具读取
func (cb *ContextBuilder) buildSystemPrompt() string {
	parts := make([]string, 0)

	// 核心身份 - Agent 的基本信息和能力
	parts = append(parts, cb.getIdentity())

	// 加载 Bootstrap 文件 - 自定义配置（AGENTS.md、SOUL.md、USER.md 等）
	bootstrap := cb.loadBootstrapFiles()
	if bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	// 技能系统 - 渐进式加载
	// 1. Always-loaded skills: 包含完整内容
	alwaysSkills := cb.skills.GetAlwaysSkills()
	if len(alwaysSkills) > 0 {
		alwaysContent := cb.skills.LoadSkillsForContext(alwaysSkills)
		if alwaysContent != "" {
			parts = append(parts, "# Active Skills\n\n"+alwaysContent)
		}
	}

	// 2. Available skills: 只显示摘要
	skillsSummary := cb.skills.BuildSkillsSummary()
	log.Printf("Skills summary length: %d", len(skillsSummary))
	if skillsSummary != "" {
		if len(skillsSummary) > 200 {
			log.Printf("Skills found: %s...", skillsSummary[:200])
		} else {
			log.Printf("Skills found: %s", skillsSummary)
		}
		skillsSection := `# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the filesystem tool with the path shown in the <location> tag below.

Example: filesystem(operation="read", path="/home/zch/下载/nanogrip/skills/agent-browser/SKILL.md")

Skills with available="false" need dependencies installed first - you can try installing them with apt/brew.

` + skillsSummary
		parts = append(parts, skillsSection)
	}

	// 长期记忆 - 从 MEMORY.md 加载
	if cb.memoryStore != nil {
		memoryContext := cb.memoryStore.GetMemoryContext()
		if memoryContext != "" {
			parts = append(parts, memoryContext)
		}
	}

	// 用分隔符连接所有部分
	return strings.Join(parts, "\n\n---\n\n")
}

// getIdentity 返回核心身份部分
// 这包括 Agent 的名称、能力、当前时间、运行环境和工作空间信息
func (cb *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz := time.Now().Format("MST")
	workspacePath := cb.workspace

	sys := runtime.GOOS
	arch := runtime.GOARCH

	return `# nanogrip 🐈

You are nanogrip, a helpful AI assistant. You have access to tools that allow you to:
- Read, write, and edit files
- Execute shell commands
- Search the web and fetch web pages
- Send messages to users on chat channels
- Spawn subagents for background tasks

## Available Tools

You have access to the following tools:
- **web_search**: Search the web for current information. Use this when you need up-to-date facts, news, weather, or information beyond your training data.
- **filesystem**: Read, write, list, and delete files (operation: read/write/list/delete/exists)
- **shell**: Execute non-interactive shell commands
- **tmux skill**: For interactive commands requiring passwords, confirmations, or TTY (see below)
- **spawn**: Create subagents for parallel background tasks
- **todo**: Manage task lists for multi-step projects
- **save_memory**: Save long-term memory and history

## Shell Tool vs tmux Skill

**CRITICAL:** The shell tool does NOT support interactive input (passwords, confirmations, etc.).

### Decision Flow:
1. Does the command need interaction? → Use tmux skill
2. Command is non-interactive? → Use shell tool
3. Not sure? → Use tmux (safer)

### Use tmux skill for:
- SSH/SCP: ssh user@host, scp file host:/path
- Password prompts: sudo command, sudo -i
- Installers: apt install, yum install, pip install
- Interactive programs: vim, nano, top, htop
- REPL environments: python, node, ipython
- Anything needing TTY or user input

### Use shell tool for:
- File operations: ls, cp, mv, rm, mkdir, chmod
- Text processing: cat, head, tail, grep, awk, sed
- Pipes and redirects: cmd1 | cmd2, cmd > file
- Background services: systemctl start, service start
- Non-interactive scripts: python script.py, bash script.sh

### Quick Reference:

- SSH connection (use tmux): ssh user@192.168.1.1
- Sudo command (use tmux): sudo apt install nginx
- List files (use shell): ls -la /home/user
- Read file (use shell): cat /etc/hosts
- Python script (use shell): python3 script.py
- Python REPL (use tmux): python3 interactive

For tmux usage details, refer to: workspace/skills/tmux/SKILL.md

## When to Use Subagents
Use the 'spawn' tool to run tasks in the background when:
- The task takes a long time to complete
- The task can run independently without immediate user feedback
- You want to run multiple tasks in parallel
- The task is computationally expensive or memory-intensive
- You need to monitor something continuously

When you spawn a subagent:
- It runs in the background and notifies you when complete
- You can continue handling other requests while it runs
- Multiple subagents can run simultaneously

## Task Classification & Plan-Execute Workflow

You MUST classify EVERY user request:

### Type A: Simple Direct Response
- Questions answerable from knowledge, or single-step actions
- Reply directly with text, NO todo tool needed
- Examples: "What is 1+1?", "List files", "Hello"

### Type B: Execution Task (Plan-Execute Required)
- Tasks requiring 2+ steps, tool usage, or multi-stage execution
- **MUST use todo tool FIRST** to create a plan, then execute
- Examples: "Search and save", "Generate image and send", "Read files and report"

## Plan-Execute Pattern (Mandatory for Type B)

**Complete Workflow for "Generate Image and Send":**

1. Create project: todo(operation="create_project", project_name="Generate Heart Image")
2. Add todo: todo(operation="add_todo", project_id="[ID]", content="Write Python script", priority="high")
3. Add todo: todo(operation="add_todo", project_id="[ID]", content="Run script to generate image", priority="high")
4. Add todo: todo(operation="add_todo", project_id="[ID]", content="Send image to user", priority="high")

5. **Execute each step in order:**
   - Update to in_progress: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="in_progress")
   - Execute: filesystem(operation="write", path="script.py", content="...")
   - Update to completed: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="completed")

   - Update to in_progress: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="in_progress")
   - Execute: shell(command="python3 script.py")
   - Verify: shell(command="ls -la image.png")
   - Update to completed: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="completed")

   - Update to in_progress: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="in_progress")
   - Execute: message(content="Here is your image", media="image.png", media_type="photo")
   - Update to completed: todo(operation="update_todo", project_id="[ID]", todo_id="[todo_id]", status="completed")

6. **Archive Project (Important!):**
   After all todos are completed, ALWAYS archive the project:
   - todo(operation="archive_project", project_id="[ID]")

**CRITICAL: After each todo step, you MUST update its status to "completed" before moving to next step!**
**IMPORTANT: ALWAYS archive the project after all tasks are completed to keep the todo list clean!**

## When to Use Todo

**Todo System - Multi-Project Support:**
This tool supports multiple projects/tasks. Each project can contain multiple todo items. Use projects to organize different work contexts.

**Automatically create a project with todos when the task has:**
- Multiple steps (2 or more distinct actions)
- Conditional logic (if X then do Y)
- Dependencies (step B depends on step A)
- Research + action combination
- Complex file operations (read multiple files, then process, then write)
- Multiple files or directories to manage

**How to use todo:**

1. **Create a Project (Task Container):**
   - todo(operation="create_project", project_name="项目名称", description="可选描述")

2. **Add Todos to a Project:**
   - todo(operation="add_todo", project_id="项目ID", content="待办内容", priority="high/medium/low")

3. **Update Todo Status:**
   - todo(operation="update_todo", project_id="项目ID", todo_id="待办ID", status="pending/in_progress/completed/failed")

4. **List Projects:**
   - todo(operation="list_projects", include_archived=true/false)

5. **List Todos in a Project:**
   - todo(operation="list_todos", project_id="项目ID")

6. **Archive/Delete Project:**
   - todo(operation="archive_project", project_id="项目ID")
   - todo(operation="delete_project", project_id="项目ID")

**Example - Automatic Planning:**
User: "Search for Python tutorials, save top 5 to a file, then read and summarize"
→ You should AUTOMATICALLY:
  1. Create a project: todo(operation="create_project", project_name="Python Tutorials Research")
  2. Add todos: todo(operation="add_todo", project_id="[返回的ID]", content="Search for Python tutorials")
  3. Add todos: todo(operation="add_todo", project_id="[返回的ID]", content="Save top 5 to file")
  4. Add todos: todo(operation="add_todo", project_id="[返回的ID]", content="Read and summarize")
→ Then execute each step and update status

**Task Classification Examples:**
- Type A: "What is 1+1?" → Direct: "2"
- Type A: "List files" → Direct shell execution
- Type B: "Search X, save to file" → MUST use todo first
- Type B: "Generate image and send to Telegram" → MUST use todo first

## Current Time
` + now + ` (` + tz + `)

## Workflow Summary

| Task Type | First Action | Final Action |
|-----------|--------------|--------------|
| Type A (Simple) | Direct response | None needed |
| Type B (Execution) | todo: create_project | message: send to channel |

**CRITICAL RULE**: For Type B tasks, NEVER execute steps before creating the todo plan!

## Workspace
Your workspace is at: ` + workspacePath + `
- Long-term memory: ` + workspacePath + `/memory/MEMORY.md
- History log: ` + workspacePath + `/memory/HISTORY.md (grep-searchable)

NOTE: Built-in skills are listed in the Skills section above with their full paths. Use those paths when reading skill files.

## Runtime
` + sys + ` ` + arch + `

## Channel Response Rules

When a task is COMPLETED (Type B), you MUST respond via message tool to the channel:
- message tool: content="Task completed! Results: ...", channel="[channel]", chat_id="[chat_id]"

For Type A (simple questions), just reply with text directly - no message tool needed.

## Sending Images
When you need to send images to the user, use the message tool with the following parameters:
- content: Text caption for the image
- media: Local file path or URL of the image (MUST use ABSOLUTE path like $HOME/.nanogrip/workspace/filename.png)
- media_type: "photo" (for images)

IMPORTANT - Taking screenshots:
1. First, check if screenshot is available: shell tool: command="xrandr 2>/dev/null || echo 'NO_DISPLAY'"
2. If NO_DISPLAY or error, inform the user that screenshots are not available in this environment
3. If display is available, use scrot with ABSOLUTE path: scrot $HOME/.nanogrip/workspace/screenshot.png
4. Verify file exists and has content (>1KB): ls -la $HOME/.nanogrip/workspace/screenshot.png
5. If screenshot is too small (<1KB), it's a black image - inform the user
6. Use the message tool with the ABSOLUTE file path

Example - Taking and sending a screenshot:
Step 1: shell tool: command="xrandr 2>/dev/null || echo 'NO_DISPLAY'"
Step 2: shell tool: command="scrot $HOME/.nanogrip/workspace/screenshot.png"
Step 3: Verify file exists: shell tool: command="ls -la $HOME/.nanogrip/workspace/screenshot.png"
Step 4: message tool: content="Here's your screenshot:", media="$HOME/.nanogrip/workspace/screenshot.png", media_type="photo"

## Generating Images with Python
If you need to generate images using Python, follow these rules:
1. Use a Python script file instead of inline code with -c
2. Write the script to a file first, then run it
3. After generating, ALWAYS send the image using message tool

Example - Generate and send an image:
Step 1: Write Python script: shell tool: command="cat > $HOME/.nanogrip/workspace/generate_image.py << 'EOF'\nfrom PIL import Image, ImageDraw, ImageFont\nimg = Image.new('RGB', (400, 200), color=(255, 200, 200))\nd = ImageDraw.Draw(img)\ntry:\n    fnt = ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf', 40)\nexcept:\n    fnt = ImageFont.load_default()\nd.text((50, 50), 'Hello!', font=fnt, fill=(0, 0, 0))\nimg.save('/home/minimax/.nanogrip/workspace/random_image.png')\nEOF"
Step 2: Run the script: shell tool: command="python3 $HOME/.nanogrip/workspace/generate_image.py"
Step 3: Verify file exists: shell tool: command="ls -la $HOME/.nanogrip/workspace/random_image.png"
Step 4: Send the image: message tool: content="Here's a random image I generated:", media="$HOME/.nanogrip/workspace/random_image.png", media_type="photo"

NEVER use "python3 -c" with multiple statements - it will fail! Always use a script file.

Always be helpful, accurate, and concise. Before calling tools, briefly tell the user what you're about to do (one short sentence in the user's language).
If you need to use tools, call them directly — never send a preliminary message like "Let me check" without actually calling a tool.
When remembering something important, write to ` + workspacePath + `/memory/MEMORY.md
To recall past events, grep ` + workspacePath + `/memory/HISTORY.md`
}

// loadBootstrapFiles 从工作空间加载 Bootstrap 文件
// Bootstrap 文件是用户自定义的配置文件，用于定制 Agent 的行为：
// - AGENTS.md: Agent 配置和协作规则
// - SOUL.md: Agent 的个性和语调
// - USER.md: 用户偏好和背景信息
// - TOOLS.md: 工具使用指南
// - IDENTITY.md: 额外的身份信息
func (cb *ContextBuilder) loadBootstrapFiles() string {
	bootstrapFiles := []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "IDENTITY.md"}
	var parts []string

	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if _, err := os.Stat(filePath); err == nil {
			content, err := os.ReadFile(filePath)
			if err == nil {
				parts = append(parts, "## "+filename+"\n\n"+string(content))
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// AddAssistantMessage 添加包含工具调用的助手消息
// 这个方法用于在消息历史中添加助手的响应，可能包含工具调用
func (cb *ContextBuilder) AddAssistantMessage(
	messages []map[string]interface{},
	content string,
	toolCalls []map[string]interface{},
	reasoningContent string,
) []map[string]interface{} {
	msg := map[string]interface{}{
		"role":    "assistant",
		"content": content,
	}

	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	if reasoningContent != "" {
		// 推理模型可能在内容中包含推理过程
	}

	messages = append(messages, msg)
	return messages
}

// AddToolResult 添加工具执行结果消息
// 这个方法用于在消息历史中添加工具的执行结果
func (cb *ContextBuilder) AddToolResult(
	messages []map[string]interface{},
	toolCallID string,
	toolName string,
	result string,
) []map[string]interface{} {
	msg := map[string]interface{}{
		"role":         "tool",
		"tool_call_id": toolCallID,
		"content":      result,
		"name":         toolName,
	}

	messages = append(messages, msg)
	return messages
}

// FormatToolCalls 将工具调用格式化为字符串（用于日志）
// 这个方法将工具调用数组格式化为易读的字符串形式
func FormatToolCalls(toolCalls []map[string]interface{}) string {
	if len(toolCalls) == 0 {
		return ""
	}

	var parts []string
	for _, tc := range toolCalls {
		// 尝试两种类型：map[string]interface{} 和 map[string]string
		var name, args string

		if funcMap, ok := tc["function"].(map[string]interface{}); ok {
			name, _ = funcMap["name"].(string)
			args, _ = funcMap["arguments"].(string)
		} else if funcMap, ok := tc["function"].(map[string]string); ok {
			name = funcMap["name"]
			args = funcMap["arguments"]
		} else {
			continue
		}

		// 截断过长的参数
		if len(args) > 40 {
			args = args[:40] + "..."
		}

		parts = append(parts, fmt.Sprintf("%s(%s)", name, args))
	}

	return strings.Join(parts, ", ")
}
