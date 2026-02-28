package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Project - 项目/任务容器
// ============================================================

// Project 表示一个项目（待办清单容器）
type Project struct {
	ID          string       `json:"id"`          // UUID
	Name        string       `json:"name"`        // 项目名称
	Description string       `json:"description"` // 项目描述（可选）
	Status      string       `json:"status"`      // 状态: active, archived, deleted
	CreatedAt   time.Time    `json:"created_at"`  // 创建时间
	UpdatedAt   time.Time    `json:"updated_at"`  // 更新时间
	Stats       ProjectStats `json:"stats"`       // 统计信息
}

// ProjectStats 项目的统计信息
type ProjectStats struct {
	Total      int `json:"total"`       // 总数
	Completed  int `json:"completed"`   // 已完成
	Pending    int `json:"pending"`     // 待处理
	InProgress int `json:"in_progress"` // 进行中
	Failed     int `json:"failed"`      // 失败
}

// ============================================================
// TodoItem - 待办事项
// ============================================================

// TodoItem 表示一个待办事项
type TodoItem struct {
	ID          string    `json:"id"`           // 唯一标识
	Content     string    `json:"content"`      // 待办内容
	Status      string    `json:"status"`       // 状态: pending, in_progress, completed, failed
	Priority    string    `json:"priority"`     // 优先级: high, medium, low
	CreatedAt   time.Time `json:"created_at"`   // 创建时间
	UpdatedAt   time.Time `json:"updated_at"`   // 更新时间
	CompletedAt time.Time `json:"completed_at"` // 完成时间（可选）
}

// TodoListData 单一项目的待办列表数据
type TodoListData struct {
	ProjectID   string     `json:"project_id"`   // 项目ID
	ProjectName string     `json:"project_name"` // 项目名称（冗余存储，便于查看）
	Timestamp   time.Time  `json:"timestamp"`    // 最后更新时间
	Todos       []TodoItem `json:"todos"`        // 待办列表
}

// ManifestData 项目索引数据
type ManifestData struct {
	Version   string    `json:"version"`    // 格式版本
	UpdatedAt time.Time `json:"updated_at"` // 最后更新时间
	Projects  []Project `json:"projects"`   // 项目列表
}

// ============================================================
// TodoTool - 多项目待办事项工具
// ============================================================

// TodoTool 提供多项目待办事项管理功能
// 支持：项目创建、归档、删除，以及项目内的待办管理
//
// 设计理念（参考 LangChain Agentic Plan-Execute 模式）：
//   - Plan：Agent 可以创建待办项目来规划复杂任务
//   - Execute：逐步执行每个待办项
//   - Track：通过状态跟踪任务进度（pending/in_progress/completed/failed）
//   - Review：完成后更新状态，便于回顾和反思
type TodoTool struct {
	BaseTool
	// workspace 是工作空间路径，用于存储待办文件
	workspace string
}

// NewTodoTool 创建一个新的多项目待办事项工具
func NewTodoTool(workspace string) *TodoTool {
	return &TodoTool{
		BaseTool: NewBaseTool(
			"todo",
			"待办事项管理工具（Agentic Task Manager）- 支持多项目/多任务的规划、执行和跟踪。\n\n设计理念：\n- Plan（规划）：创建项目和待办来规划复杂任务\n- Execute（执行）：逐步执行每个待办项\n- Track（跟踪）：通过状态跟踪任务进度\n- Review（回顾）：完成后更新状态\n\n可用操作：\n- create_project: 创建新项目\n- list_projects: 列出所有项目\n- add_todo: 添加待办事项\n- update_todo: 更新待办状态\n- list_todos: 列出项目中的待办\n- archive_project: 归档项目\n- delete_project: 删除项目",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"operation": map[string]interface{}{
						"type":        "string",
						"description": "操作类型: create_project(创建项目), list_projects(列出项目), archive_project(归档项目), delete_project(删除项目), add_todo(添加待办), list_todos(列出待办), update_todo(更新待办), delete_todo(删除待办)",
					},
					// 项目操作参数
					"project_name": map[string]interface{}{
						"type":        "string",
						"description": "项目名称（create_project时必需）",
					},
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "项目ID（archive/delete/list_todos/add_todo/update_todo/delete_todo时必需）",
					},
					// 待办操作参数
					"content": map[string]interface{}{
						"type":        "string",
						"description": "待办内容（add_todo时必需）",
					},
					"todo_id": map[string]interface{}{
						"type":        "string",
						"description": "待办ID（update_todo/delete_todo时必需）",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "状态: pending(待处理), in_progress(进行中), completed(已完成), failed(失败)",
					},
					"priority": map[string]interface{}{
						"type":        "string",
						"description": "优先级: high(高), medium(中), low(低)，默认medium",
					},
					"include_archived": map[string]interface{}{
						"type":        "boolean",
						"description": "是否包含已归档的项目，默认为false",
					},
				},
				"required": []string{"operation"},
			},
		),
		workspace: workspace,
	}
}

// getDirs 获取目录路径
func (t *TodoTool) getDirs() (string, string, string) {
	baseDir := filepath.Join(t.workspace, "todos")
	currentDir := filepath.Join(baseDir, "current")
	archiveDir := filepath.Join(baseDir, "archive")
	return baseDir, currentDir, archiveDir
}

// getManifestPath 获取项目索引文件路径
func (t *TodoTool) getManifestPath() string {
	baseDir, _, _ := t.getDirs()
	return filepath.Join(baseDir, "manifest.json")
}

// getProjectFilePath 获取项目待办文件路径
func (t *TodoTool) getProjectFilePath(projectID string, archived bool) string {
	_, currentDir, archiveDir := t.getDirs()
	if archived {
		return filepath.Join(archiveDir, fmt.Sprintf("%s.json", projectID))
	}
	return filepath.Join(currentDir, fmt.Sprintf("%s.json", projectID))
}

// ensureDirs 确保目录存在
func (t *TodoTool) ensureDirs() error {
	baseDir, currentDir, archiveDir := t.getDirs()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return err
	}
	return nil
}

// loadManifest 加载项目索引
func (t *TodoTool) loadManifest() (*ManifestData, error) {
	manifestPath := t.getManifestPath()

	// 确保目录存在
	if err := t.ensureDirs(); err != nil {
		return &ManifestData{Version: "1.0", Projects: []Project{}}, nil
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// 文件不存在，返回空索引
		return &ManifestData{Version: "1.0", Projects: []Project{}}, nil
	}

	var manifest ManifestData
	if err := json.Unmarshal(data, &manifest); err != nil {
		return &ManifestData{Version: "1.0", Projects: []Project{}}, nil
	}

	return &manifest, nil
}

// saveManifest 保存项目索引
func (t *TodoTool) saveManifest(manifest *ManifestData) error {
	manifestPath := t.getManifestPath()
	manifest.UpdatedAt = time.Now()

	jsonData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(manifestPath, jsonData, 0644)
}

// loadProjectTodos 加载项目的待办列表
func (t *TodoTool) loadProjectTodos(projectID string) (*TodoListData, error) {
	// 先尝试当前目录
	filePath := t.getProjectFilePath(projectID, false)
	data, err := os.ReadFile(filePath)
	if err == nil {
		var todoData TodoListData
		if err := json.Unmarshal(data, &todoData); err != nil {
			return &TodoListData{ProjectID: projectID, Todos: []TodoItem{}}, nil
		}
		return &todoData, nil
	}

	// 尝试归档目录
	filePath = t.getProjectFilePath(projectID, true)
	data, err = os.ReadFile(filePath)
	if err != nil {
		return &TodoListData{ProjectID: projectID, Todos: []TodoItem{}}, nil
	}

	var todoData TodoListData
	if err := json.Unmarshal(data, &todoData); err != nil {
		return &TodoListData{ProjectID: projectID, Todos: []TodoItem{}}, nil
	}

	return &todoData, nil
}

// saveProjectTodos 保存项目的待办列表
func (t *TodoTool) saveProjectTodos(data *TodoListData) error {
	// 检查项目状态以确定保存位置
	manifest, err := t.loadManifest()
	if err != nil {
		return err
	}

	archived := false
	for _, p := range manifest.Projects {
		if p.ID == data.ProjectID && p.Status == "archived" {
			archived = true
			break
		}
	}

	filePath := t.getProjectFilePath(data.ProjectID, archived)
	data.Timestamp = time.Now()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, jsonData, 0644)
}

// updateProjectStats 更新项目的统计信息
func (t *TodoTool) updateProjectStats(projectID string) error {
	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return err
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return err
	}

	stats := ProjectStats{
		Total: len(todoData.Todos),
	}
	for _, todo := range todoData.Todos {
		switch todo.Status {
		case "completed":
			stats.Completed++
		case "pending":
			stats.Pending++
		case "in_progress":
			stats.InProgress++
		case "failed":
			stats.Failed++
		}
	}

	for i := range manifest.Projects {
		if manifest.Projects[i].ID == projectID {
			manifest.Projects[i].Stats = stats
			manifest.Projects[i].UpdatedAt = time.Now()
			break
		}
	}

	return t.saveManifest(manifest)
}

// Execute 执行待办事项操作
func (t *TodoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 确保目录存在
	if err := t.ensureDirs(); err != nil {
		return fmt.Sprintf(`{"error": "创建目录失败: %v"}`, err), nil
	}

	// 获取操作类型
	operation, _ := params["operation"].(string)
	if operation == "" {
		return `{"error": "operation 是必需参数"}`, nil
	}

	// 执行对应的操作
	switch operation {
	case "create_project":
		return t.handleCreateProject(params)
	case "list_projects":
		return t.handleListProjects(params)
	case "archive_project":
		return t.handleArchiveProject(params)
	case "delete_project":
		return t.handleDeleteProject(params)
	case "add_todo":
		return t.handleAddTodo(params)
	case "list_todos":
		return t.handleListTodos(params)
	case "update_todo":
		return t.handleUpdateTodo(params)
	case "delete_todo":
		return t.handleDeleteTodo(params)
	default:
		return fmt.Sprintf(`{"error": "未知操作: %s，有效操作: create_project, list_projects, archive_project, delete_project, add_todo, list_todos, update_todo, delete_todo"}`, operation), nil
	}
}

// handleCreateProject 处理创建项目
func (t *TodoTool) handleCreateProject(params map[string]interface{}) (string, error) {
	name, _ := params["project_name"].(string)
	if name == "" {
		return `{"error": "project_name 是创建项目的必需参数"}`, nil
	}

	description, _ := params["description"].(string)

	// 生成UUID
	projectID := uuid.New().String()

	// 创建项目
	project := Project{
		ID:          projectID,
		Name:        name,
		Description: description,
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Stats:       ProjectStats{},
	}

	// 加载并更新索引
	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	manifest.Projects = append(manifest.Projects, project)

	if err := t.saveManifest(manifest); err != nil {
		return fmt.Sprintf(`{"error": "保存索引失败: %v"}`, err), nil
	}

	// 创建空的待办文件
	todoData := TodoListData{
		ProjectID:   projectID,
		ProjectName: name,
		Timestamp:   time.Now(),
		Todos:       []TodoItem{},
	}
	if err := t.saveProjectTodos(&todoData); err != nil {
		return fmt.Sprintf(`{"error": "创建待办文件失败: %v"}`, err), nil
	}

	return fmt.Sprintf(`{"status": "created", "project_id": "%s", "project_name": "%s"}`,
		projectID, name), nil
}

// handleListProjects 处理列出项目
func (t *TodoTool) handleListProjects(params map[string]interface{}) (string, error) {
	includeArchived, _ := params["include_archived"].(bool)

	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	if len(manifest.Projects) == 0 {
		return "# 项目列表\n\n暂无项目，请使用 create_project 创建新项目", nil
	}

	result := "# 项目列表\n\n"

	activeProjects := []Project{}
	archivedProjects := []Project{}

	for _, p := range manifest.Projects {
		if p.Status == "archived" {
			archivedProjects = append(archivedProjects, p)
		} else if p.Status != "deleted" {
			activeProjects = append(activeProjects, p)
		}
	}

	// 显示活跃项目
	if len(activeProjects) > 0 {
		result += "## 🔵 进行中\n\n"
		for _, p := range activeProjects {
			progress := 0
			if p.Stats.Total > 0 {
				progress = (p.Stats.Completed * 100) / p.Stats.Total
			}
			result += fmt.Sprintf("- **[%s]** %s (进度: %d%%, %d/%d)\n",
				p.Status, p.Name, progress, p.Stats.Completed, p.Stats.Total)
			if p.Description != "" {
				result += fmt.Sprintf("  - %s\n", p.Description)
			}
		}
		result += "\n"
	}

	// 显示归档项目
	if includeArchived && len(archivedProjects) > 0 {
		result += "## 📦 已归档\n\n"
		for _, p := range archivedProjects {
			result += fmt.Sprintf("- [%s] %s (%d/%d)\n",
				p.Status, p.Name, p.Stats.Completed, p.Stats.Total)
		}
		result += "\n"
	}

	// 统计
	result += fmt.Sprintf("---\n**统计**: 共 %d 个项目 | 活跃 %d | 归档 %d",
		len(manifest.Projects), len(activeProjects), len(archivedProjects))

	return result, nil
}

// handleArchiveProject 处理归档项目
func (t *TodoTool) handleArchiveProject(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是归档项目的必需参数"}`, nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	// 查找并更新项目状态
	found := false
	projectName := ""
	for i := range manifest.Projects {
		if manifest.Projects[i].ID == projectID {
			if manifest.Projects[i].Status == "archived" {
				return fmt.Sprintf(`{"error": "项目已归档: %s"}`, projectID), nil
			}
			manifest.Projects[i].Status = "archived"
			manifest.Projects[i].UpdatedAt = time.Now()
			projectName = manifest.Projects[i].Name
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到项目: %s"}`, projectID), nil
	}

	if err := t.saveManifest(manifest); err != nil {
		return fmt.Sprintf(`{"error": "保存索引失败: %v"}`, err), nil
	}

	// 移动文件到归档目录
	currentPath := t.getProjectFilePath(projectID, false)
	archivePath := t.getProjectFilePath(projectID, true)

	if err := os.Rename(currentPath, archivePath); err != nil {
		// 如果文件不存在，可能已经移动过了，忽略错误
	}

	return fmt.Sprintf(`{"status": "archived", "project_id": "%s", "project_name": "%s"}`,
		projectID, projectName), nil
}

// handleDeleteProject 处理删除项目
func (t *TodoTool) handleDeleteProject(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是删除项目的必需参数"}`, nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	// 查找并标记删除
	found := false
	projectName := ""
	for i := range manifest.Projects {
		if manifest.Projects[i].ID == projectID {
			manifest.Projects[i].Status = "deleted"
			manifest.Projects[i].UpdatedAt = time.Now()
			projectName = manifest.Projects[i].Name
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到项目: %s"}`, projectID), nil
	}

	if err := t.saveManifest(manifest); err != nil {
		return fmt.Sprintf(`{"error": "保存索引失败: %v"}`, err), nil
	}

	// 删除待办文件（当前和归档）
	currentPath := t.getProjectFilePath(projectID, false)
	archivePath := t.getProjectFilePath(projectID, true)
	os.Remove(currentPath)
	os.Remove(archivePath)

	return fmt.Sprintf(`{"status": "deleted", "project_id": "%s", "project_name": "%s"}`,
		projectID, projectName), nil
}

// handleAddTodo 处理添加待办
func (t *TodoTool) handleAddTodo(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是添加待办的必需参数"}`, nil
	}

	content, _ := params["content"].(string)
	if content == "" {
		return `{"error": "content 是添加待办的必需参数"}`, nil
	}

	// 获取优先级
	priority, _ := params["priority"].(string)
	if priority == "" {
		priority = "medium"
	}
	if priority != "high" && priority != "medium" && priority != "low" {
		priority = "medium"
	}

	// 验证项目存在
	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	projectName := ""
	found := false
	for _, p := range manifest.Projects {
		if p.ID == projectID && p.Status != "deleted" {
			projectName = p.Name
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到项目: %s"}`, projectID), nil
	}

	// 加载待办
	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return fmt.Sprintf(`{"error": "加载待办失败: %v"}`, err), nil
	}

	// 创建待办
	todo := TodoItem{
		ID:        fmt.Sprintf("todo-%d", time.Now().UnixNano()),
		Content:   content,
		Status:    "pending",
		Priority:  priority,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	todoData.Todos = append(todoData.Todos, todo)
	todoData.ProjectName = projectName

	if err := t.saveProjectTodos(todoData); err != nil {
		return fmt.Sprintf(`{"error": "保存待办失败: %v"}`, err), nil
	}

	// 更新统计
	t.updateProjectStats(projectID)

	return fmt.Sprintf(`{"status": "added", "todo_id": "%s", "project_id": "%s", "content": "%s", "priority": "%s"}`,
		todo.ID, projectID, content, priority), nil
}

// handleListTodos 处理列出待办
func (t *TodoTool) handleListTodos(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是列出待办的必需参数"}`, nil
	}

	// 验证项目存在
	manifest, err := t.loadManifest()
	if err != nil {
		return fmt.Sprintf(`{"error": "加载索引失败: %v"}`, err), nil
	}

	projectName := ""
	found := false
	for _, p := range manifest.Projects {
		if p.ID == projectID && p.Status != "deleted" {
			projectName = p.Name
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到项目: %s"}`, projectID), nil
	}

	// 加载待办
	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return fmt.Sprintf(`{"error": "加载待办失败: %v"}`, err), nil
	}

	if len(todoData.Todos) == 0 {
		return fmt.Sprintf("# 项目待办: %s\n\n暂无待办事项", projectName), nil
	}

	result := fmt.Sprintf("# 项目待办: %s\n\n", projectName)

	// 按状态分组
	pending := []TodoItem{}
	inProgress := []TodoItem{}
	completed := []TodoItem{}
	failed := []TodoItem{}

	for _, todo := range todoData.Todos {
		switch todo.Status {
		case "pending":
			pending = append(pending, todo)
		case "in_progress":
			inProgress = append(inProgress, todo)
		case "completed":
			completed = append(completed, todo)
		case "failed":
			failed = append(failed, todo)
		}
	}

	// 输出
	if len(inProgress) > 0 {
		result += "## 🔄 进行中\n"
		for _, todo := range inProgress {
			icon := "📌"
			if todo.Priority == "high" {
				icon = "🔴"
			} else if todo.Priority == "low" {
				icon = "🟢"
			}
			result += fmt.Sprintf("- %s **[%s]** %s\n", icon, todo.Status, todo.Content)
		}
		result += "\n"
	}

	if len(pending) > 0 {
		result += "## ⏳ 待处理\n"
		for _, todo := range pending {
			icon := "📌"
			if todo.Priority == "high" {
				icon = "🔴"
			} else if todo.Priority == "low" {
				icon = "🟢"
			}
			result += fmt.Sprintf("- %s **[%s]** %s\n", icon, todo.Status, todo.Content)
		}
		result += "\n"
	}

	if len(completed) > 0 {
		result += "## ✅ 已完成\n"
		for _, todo := range completed {
			result += fmt.Sprintf("- ~~%s~~\n", todo.Content)
		}
		result += "\n"
	}

	if len(failed) > 0 {
		result += "## ❌ 失败\n"
		for _, todo := range failed {
			result += fmt.Sprintf("- **%s** %s\n", todo.Status, todo.Content)
		}
		result += "\n"
	}

	result += fmt.Sprintf("---\n**统计**: 总计 %d | 进行中 %d | 待处理 %d | 已完成 %d | 失败 %d",
		len(todoData.Todos), len(inProgress), len(pending), len(completed), len(failed))

	return result, nil
}

// handleUpdateTodo 处理更新待办
func (t *TodoTool) handleUpdateTodo(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是更新待办的必需参数"}`, nil
	}

	todoID, _ := params["todo_id"].(string)
	if todoID == "" {
		return `{"error": "todo_id 是更新待办的必需参数"}`, nil
	}

	status, _ := params["status"].(string)
	if status == "" {
		return `{"error": "status 是更新待办的必需参数"}`, nil
	}

	// 验证状态
	validStatuses := map[string]bool{
		"pending":     true,
		"in_progress": true,
		"completed":   true,
		"failed":      true,
	}
	if !validStatuses[status] {
		return `{"error": "无效的状态，请使用: pending, in_progress, completed, failed"}`, nil
	}

	// 加载待办
	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return fmt.Sprintf(`{"error": "加载待办失败: %v"}`, err), nil
	}

	// 查找并更新
	found := false
	for i := range todoData.Todos {
		if todoData.Todos[i].ID == todoID {
			todoData.Todos[i].Status = status
			todoData.Todos[i].UpdatedAt = time.Now()
			if status == "completed" {
				todoData.Todos[i].CompletedAt = time.Now()
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到待办: %s"}`, todoID), nil
	}

	if err := t.saveProjectTodos(todoData); err != nil {
		return fmt.Sprintf(`{"error": "保存待办失败: %v"}`, err), nil
	}

	// 更新统计
	t.updateProjectStats(projectID)

	return fmt.Sprintf(`{"status": "updated", "todo_id": "%s", "project_id": "%s", "new_status": "%s"}`,
		todoID, projectID, status), nil
}

// handleDeleteTodo 处理删除待办
func (t *TodoTool) handleDeleteTodo(params map[string]interface{}) (string, error) {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return `{"error": "project_id 是删除待办的必需参数"}`, nil
	}

	todoID, _ := params["todo_id"].(string)
	if todoID == "" {
		return `{"error": "todo_id 是删除待办的必需参数"}`, nil
	}

	// 加载待办
	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return fmt.Sprintf(`{"error": "加载待办失败: %v"}`, err), nil
	}

	// 查找并删除
	found := false
	var newTodos []TodoItem
	for _, todo := range todoData.Todos {
		if todo.ID == todoID {
			found = true
			continue
		}
		newTodos = append(newTodos, todo)
	}

	if !found {
		return fmt.Sprintf(`{"error": "未找到待办: %s"}`, todoID), nil
	}

	todoData.Todos = newTodos

	if err := t.saveProjectTodos(todoData); err != nil {
		return fmt.Sprintf(`{"error": "保存待办失败: %v"}`, err), nil
	}

	// 更新统计
	t.updateProjectStats(projectID)

	return fmt.Sprintf(`{"status": "deleted", "todo_id": "%s", "project_id": "%s"}`,
		todoID, projectID), nil
}
