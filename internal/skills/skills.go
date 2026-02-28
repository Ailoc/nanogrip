// Package skills 提供技能加载和管理功能
//
// 技能系统允许代理(agent)动态加载和使用外部功能。每个技能是一个包含 SKILL.md 文件的目录，
// 该文件使用 YAML frontmatter 格式存储元数据，并包含技能的说明文档。
//
// 技能来源优先级：
//  1. workspace/skills - 工作区技能(最高优先级，会覆盖内置技能)
//  2. builtin skills - 内置技能
//
// YAML frontmatter 格式示例：
//
//	---
//	name: "skill-name"
//	description: "技能描述"
//	metadata: '{"nanobot":{"requires":{"bins":["git"],"env":["API_KEY"]}}}'
//	always: true
//	---
//
// metadata 字段包含 JSON 格式的需求定义：
//   - bins: 需要的命令行工具列表
//   - env: 需要的环境变量列表
//
// 只有满足所有需求的技能才会被标记为 available=true
package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Skill 表示一个技能及其元数据和内容
// 每个技能包含名称、路径、来源、内容、元数据、可用性和描述
type Skill struct {
	Name        string        // 技能名称（目录名）
	Path        string        // SKILL.md 文件的完整路径
	Source      string        // 技能来源: "workspace" 或 "builtin"
	Content     string        // SKILL.md 文件的完整内容（包含 frontmatter）
	Metadata    SkillMetadata // 从 frontmatter 解析的元数据
	Available   bool          // 是否满足所有需求（bins 和 env）
	Description string        // 技能描述（用于显示）
}

// SkillMetadata 表示从 SKILL.md frontmatter 解析的元数据
//
// Frontmatter 是在 Markdown 文件开头用 --- 包围的 YAML 格式元数据块。
// 示例：
//
//	---
//	name: git-ops
//	description: Git operations skill
//	metadata: '{"nanobot":{"requires":{"bins":["git"]}}}'
//	always: true
//	---
type SkillMetadata struct {
	Name        string            `yaml:"name"`        // 技能名称
	Description string            `yaml:"description"` // 技能描述
	Metadata    string            `yaml:"metadata"`    // JSON 字符串，包含 nanobot 配置
	Always      bool              // 如果为 true，技能会始终加载到代理上下文
	Requires    SkillRequirements `yaml:"-"` // 从 Metadata JSON 解析的需求
}

// SkillRequirements 定义技能运行所需的依赖
// 包括命令行工具和环境变量
type SkillRequirements struct {
	Bins []string `json:"bins"` // 需要的命令行工具（例如：git, docker, kubectl）
	Env  []string `json:"env"`  // 需要的环境变量（例如：API_KEY, GITHUB_TOKEN）
}

// SkillsLoader 负责加载和管理代理技能
//
// 工作流程：
//  1. 从 workspace/skills 和 builtin skills 目录扫描技能
//  2. 解析每个技能的 SKILL.md 文件的 YAML frontmatter
//  3. 检查技能需求是否满足（bins 和 env）
//  4. 缓存已加载的技能以提高性能
type SkillsLoader struct {
	workspace       string                    // 工作区根目录路径
	workspaceSkills string                    // 工作区技能目录路径（workspace/skills）
	builtinSkills   string                    // 内置技能目录路径
	skillsCache     map[string]*Skill         // 技能缓存，键为 "source:name"
	metadataCache   map[string]*SkillMetadata // 元数据缓存（当前未使用）
}

// NewSkillsLoader 创建一个新的技能加载器
//
// 参数：
//   - workspace: 工作区根目录路径
//   - builtinSkills: 内置技能目录路径
//
// 返回：
//   - *SkillsLoader: 技能加载器实例
func NewSkillsLoader(workspace string, builtinSkills string) *SkillsLoader {
	return &SkillsLoader{
		workspace:       workspace,
		workspaceSkills: filepath.Join(workspace, "skills"),
		builtinSkills:   builtinSkills,
		skillsCache:     make(map[string]*Skill),
		metadataCache:   make(map[string]*SkillMetadata),
	}
}

// ListSkills 列出所有技能
//
// 加载顺序：
//  1. 首先加载工作区技能（workspace/skills）
//  2. 然后加载内置技能（builtin skills）
//  3. 如果工作区技能和内置技能同名，工作区技能优先
//
// 参数：
//   - filterUnavailable: 如果为 true，只返回 available=true 的技能
//
// 返回：
//   - []*Skill: 技能列表
func (s *SkillsLoader) ListSkills(filterUnavailable bool) []*Skill {
	var result []*Skill

	// Workspace skills (highest priority)
	if _, err := os.Stat(s.workspaceSkills); err == nil {
		entries, _ := os.ReadDir(s.workspaceSkills)
		for _, entry := range entries {
			if entry.IsDir() {
				skillFile := filepath.Join(s.workspaceSkills, entry.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err == nil {
					skill := s.loadSkill(entry.Name(), "workspace")
					if skill != nil {
						if !filterUnavailable || skill.Available {
							result = append(result, skill)
						} else if !filterUnavailable {
							result = append(result, skill)
						}
					}
				}
			}
		}
	}

	// Built-in skills
	if s.builtinSkills != "" {
		if _, err := os.Stat(s.builtinSkills); err == nil {
			entries, _ := os.ReadDir(s.builtinSkills)
			for _, entry := range entries {
				if entry.IsDir() {
					// Skip if already loaded from workspace
					exists := false
					for _, existing := range result {
						if existing.Name == entry.Name() {
							exists = true
							break
						}
					}
					if exists {
						continue
					}

					skillFile := filepath.Join(s.builtinSkills, entry.Name(), "SKILL.md")
					if _, err := os.Stat(skillFile); err == nil {
						skill := s.loadSkill(entry.Name(), "builtin")
						if skill != nil {
							if !filterUnavailable || skill.Available {
								result = append(result, skill)
							} else if !filterUnavailable {
								result = append(result, skill)
							}
						}
					}
				}
			}
		}
	}

	return result
}

// loadSkill 从指定来源加载一个技能
//
// 工作流程：
//  1. 检查缓存，如果已加载则直接返回
//  2. 根据 source 确定技能文件路径
//  3. 读取 SKILL.md 文件内容
//  4. 解析 YAML frontmatter 获取元数据
//  5. 检查技能需求是否满足
//  6. 缓存技能并返回
//
// 参数：
//   - name: 技能名称（目录名）
//   - source: 技能来源（"workspace" 或 "builtin"）
//
// 返回：
//   - *Skill: 加载的技能，如果加载失败返回 nil
func (s *SkillsLoader) loadSkill(name string, source string) *Skill {
	cacheKey := source + ":" + name
	if skill, ok := s.skillsCache[cacheKey]; ok {
		return skill
	}

	var skillPath string
	if source == "workspace" {
		skillPath = filepath.Join(s.workspaceSkills, name, "SKILL.md")
	} else {
		skillPath = filepath.Join(s.builtinSkills, name, "SKILL.md")
	}

	// 转换为绝对路径，确保agent可以正确读取
	absSkillPath, err := filepath.Abs(skillPath)
	if err == nil {
		skillPath = absSkillPath
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		return nil
	}

	metadata := s.parseSkillMetadata(string(content))

	skill := &Skill{
		Name:        name,
		Path:        skillPath,
		Source:      source,
		Content:     string(content),
		Metadata:    *metadata,
		Available:   s.checkRequirements(metadata),
		Description: metadata.Description,
	}

	s.skillsCache[cacheKey] = skill
	return skill
}

// LoadSkill 按名称加载一个特定技能
//
// 加载优先级：
//  1. 首先尝试从工作区加载
//  2. 如果工作区没有，再从内置技能加载
//
// 参数：
//   - name: 技能名称
//
// 返回：
//   - *Skill: 找到的技能，如果都没找到返回 nil
func (s *SkillsLoader) LoadSkill(name string) *Skill {
	// Check workspace first
	if skill := s.loadSkill(name, "workspace"); skill != nil {
		return skill
	}
	// Check builtin
	return s.loadSkill(name, "builtin")
}

// LoadSkillsForContext 加载指定的技能以包含在代理上下文中
//
// 该方法会移除 YAML frontmatter，只保留技能的说明文档部分，
// 然后将多个技能的内容组合成一个字符串，用于注入到代理的上下文。
//
// 参数：
//   - skillNames: 要加载的技能名称列表
//
// 返回：
//   - string: 组合后的技能内容，格式为:
//     ### Skill: skill-name1
//     <content1>
//     ---
//     ### Skill: skill-name2
//     <content2>
func (s *SkillsLoader) LoadSkillsForContext(skillNames []string) string {
	var parts []string
	for _, name := range skillNames {
		skill := s.LoadSkill(name)
		if skill != nil {
			content := stripFrontmatter(skill.Content)
			parts = append(parts, "### Skill: "+name+"\n\n"+content)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary 构建所有技能的摘要信息，以 XML 格式返回
//
// XML 格式示例：
//
//	<skills>
//	  <skill available="true">
//	    <name>git-ops</name>
//	    <description>Git operations</description>
//	    <location>/path/to/skill/SKILL.md</location>
//	  </skill>
//	  <skill available="false">
//	    <name>docker-ops</name>
//	    <description>Docker operations</description>
//	    <location>/path/to/skill/SKILL.md</location>
//	    <requires>CLI: docker, ENV: DOCKER_HOST</requires>
//	  </skill>
//	</skills>
//
// 该摘要用于让代理了解哪些技能可用，哪些技能因缺少依赖而不可用。
//
// 返回：
//   - string: XML 格式的技能摘要
func (s *SkillsLoader) BuildSkillsSummary() string {
	allSkills := s.ListSkills(false)
	if len(allSkills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<skills>")

	for _, skill := range allSkills {
		available := skill.Available
		lines = append(lines, "  <skill available=\""+strconv.FormatBool(available)+"\">")
		lines = append(lines, "    <name>"+escapeXML(skill.Name)+"</name>")
		lines = append(lines, "    <description>"+escapeXML(skill.Description)+"</description>")
		lines = append(lines, "    <location>"+skill.Path+"</location>")

		// Show missing requirements for unavailable skills
		if !available {
			missing := s.getMissingRequirements(&skill.Metadata)
			if missing != "" {
				lines = append(lines, "    <requires>"+escapeXML(missing)+"</requires>")
			}
		}

		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")

	return strings.Join(lines, "\n")
}

// GetAlwaysSkills 获取标记为 always=true 且满足需求的技能列表
//
// always=true 的技能会始终加载到代理上下文中，无需显式调用。
// 这对于频繁使用的基础技能很有用。
//
// 返回：
//   - []string: 应该始终加载的技能名称列表
func (s *SkillsLoader) GetAlwaysSkills() []string {
	var result []string
	for _, skill := range s.ListSkills(true) {
		if skill.Metadata.Always {
			result = append(result, skill.Name)
		}
	}
	return result
}

// parseSkillMetadata 从 SKILL.md 的 frontmatter 解析元数据
//
// YAML Frontmatter 解析过程：
//  1. 检查文件是否以 "---" 开头
//  2. 使用正则表达式提取 frontmatter 块（第一个 --- 和第二个 --- 之间的内容）
//  3. 逐行解析 YAML 键值对（使用简单的字符串分割，不依赖 YAML 库）
//  4. 解析 metadata 字段中的嵌套 JSON 结构
//  5. 从 JSON 中提取 nanobot.requires.bins 和 nanobot.requires.env
//
// YAML frontmatter 格式：
//
//	---
//	name: skill-name
//	description: "技能描述"
//	metadata: '{"nanobot":{"requires":{"bins":["git"],"env":["API_KEY"]}}}'
//	always: true
//	---
//
// JSON metadata 结构：
//
//	{
//	  "nanobot": {
//	    "requires": {
//	      "bins": ["git", "docker"],
//	      "env": ["API_KEY", "SECRET"]
//	    }
//	  }
//	}
//
// 参数：
//   - content: SKILL.md 文件的完整内容
//
// 返回：
//   - *SkillMetadata: 解析后的元数据
func (s *SkillsLoader) parseSkillMetadata(content string) *SkillMetadata {
	metadata := &SkillMetadata{}

	// Check for YAML frontmatter
	if strings.HasPrefix(content, "---") {
		re := regexp.MustCompile(`(?s)^---\n(.*?)\n---`)
		match := re.FindStringSubmatch(content)
		if match != nil {
			yamlContent := match[1]

			// Simple YAML parsing
			lines := strings.Split(yamlContent, "\n")
			for _, line := range lines {
				if strings.Contains(line, ":") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						value = strings.Trim(value, "\"")

						switch key {
						case "name":
							metadata.Name = value
						case "description":
							metadata.Description = value
						case "metadata":
							metadata.Metadata = value
						case "always":
							metadata.Always = value == "true"
						}
					}
				}
			}

			// Parse nested JSON metadata
			if metadata.Metadata != "" {
				var nanobotData map[string]interface{}
				if err := json.Unmarshal([]byte(metadata.Metadata), &nanobotData); err == nil {
					if nb, ok := nanobotData["nanobot"].(map[string]interface{}); ok {
						if requires, ok := nb["requires"].(map[string]interface{}); ok {
							if bins, ok := requires["bins"].([]interface{}); ok {
								for _, b := range bins {
									if bStr, ok := b.(string); ok {
										metadata.Requires.Bins = append(metadata.Requires.Bins, bStr)
									}
								}
							}
							if env, ok := requires["env"].([]interface{}); ok {
								for _, e := range env {
									if eStr, ok := e.(string); ok {
										metadata.Requires.Env = append(metadata.Requires.Env, eStr)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Set default description if empty
	if metadata.Description == "" {
		metadata.Description = metadata.Name
	}

	return metadata
}

// checkRequirements 检查技能需求是否满足
//
// 检查项：
//  1. Bins: 检查 PATH 中是否存在所需的命令行工具
//  2. Env: 检查是否设置了所需的环境变量
//
// 只有所有需求都满足时，技能才会被标记为 available=true
//
// 参数：
//   - metadata: 技能元数据
//
// 返回：
//   - bool: 如果所有需求都满足返回 true，否则返回 false
func (s *SkillsLoader) checkRequirements(metadata *SkillMetadata) bool {
	for _, bin := range metadata.Requires.Bins {
		if !hasCommand(bin) {
			return false
		}
	}
	for _, env := range metadata.Requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	return true
}

// getMissingRequirements 返回缺失需求的描述
//
// 用于生成用户友好的错误信息，说明技能为什么不可用。
//
// 参数：
//   - metadata: 技能元数据
//
// 返回：
//   - string: 缺失需求的描述，格式: "CLI: git, ENV: API_KEY"
func (s *SkillsLoader) getMissingRequirements(metadata *SkillMetadata) string {
	var missing []string

	for _, bin := range metadata.Requires.Bins {
		if !hasCommand(bin) {
			missing = append(missing, "CLI: "+bin)
		}
	}
	for _, env := range metadata.Requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, "ENV: "+env)
		}
	}

	return strings.Join(missing, ", ")
}

// stripFrontmatter 从 Markdown 内容中移除 YAML frontmatter
//
// 移除过程：
//  1. 检查内容是否以 "---" 开头
//  2. 使用正则表达式匹配并移除从第一个 --- 到第二个 --- 的所有内容
//  3. 返回纯净的 Markdown 文档内容
//
// 这个函数用于在将技能内容注入到代理上下文时，只保留说明文档部分。
//
// 参数：
//   - content: 包含 frontmatter 的完整内容
//
// 返回：
//   - string: 移除 frontmatter 后的内容
func stripFrontmatter(content string) string {
	if strings.HasPrefix(content, "---") {
		re := regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
		return re.ReplaceAllString(content, "")
	}
	return content
}

// escapeXML 转义 XML 特殊字符
//
// 转义以下字符以防止 XML 格式错误：
//   - & -> &amp;
//   - < -> &lt;
//   - > -> &gt;
//
// 参数：
//   - s: 原始字符串
//
// 返回：
//   - string: 转义后的字符串
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// hasCommand 检查命令是否存在于 PATH 中
//
// 检查策略：
//  1. 如果命令包含路径分隔符（/ 或 \），直接检查该路径是否存在
//  2. 否则，在 PATH 环境变量的所有目录中搜索该命令
//
// 这个函数用于验证技能需求的 bins 是否满足。
//
// 参数：
//   - cmd: 命令名称或完整路径
//
// 返回：
//   - bool: 如果命令存在返回 true，否则返回 false
func hasCommand(cmd string) bool {
	// Check if command exists using shell
	// On Unix, we can use "which" or "command -v"
	// For simplicity, we'll just check common paths
	path := os.Getenv("PATH")
	if path == "" {
		return false
	}

	// If cmd contains path separator, check directly
	if strings.Contains(cmd, "/") || strings.Contains(cmd, "\\") {
		_, err := os.Stat(cmd)
		return err == nil
	}

	// Search in PATH
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		fullPath := filepath.Join(dir, cmd)
		if _, err := os.Stat(fullPath); err == nil {
			return true
		}
	}
	return false
}
