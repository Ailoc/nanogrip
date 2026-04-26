package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	todoDataVersion = "1.0"

	todoOperationListProjects   = "list_projects"
	todoOperationAddTodos       = "add_todos"
	todoOperationListTodos      = "list_todos"
	todoOperationUpdateTodo     = "update_todo"
	todoOperationArchiveProject = "archive_project"
	todoOperationDeleteProject  = "delete_project"
	todoOperationDeleteTodo     = "delete_todo"

	projectStatusActive   = "active"
	projectStatusArchived = "archived"
	projectStatusDeleted  = "deleted"

	todoStatusPending    = "pending"
	todoStatusInProgress = "in_progress"
	todoStatusCompleted  = "completed"
	todoStatusFailed     = "failed"

	todoPriorityHigh   = "high"
	todoPriorityMedium = "medium"
	todoPriorityLow    = "low"
)

var (
	validTodoOperations = []string{
		todoOperationListProjects,
		todoOperationAddTodos,
		todoOperationListTodos,
		todoOperationUpdateTodo,
		todoOperationArchiveProject,
		todoOperationDeleteProject,
		todoOperationDeleteTodo,
	}

	validTodoStatuses = map[string]struct{}{
		todoStatusPending:    {},
		todoStatusInProgress: {},
		todoStatusCompleted:  {},
		todoStatusFailed:     {},
	}
)

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

// TodoTool 提供多项目待办事项管理功能。
type TodoTool struct {
	BaseTool
	workspace string
	mu        sync.Mutex
}

// NewTodoTool 创建一个新的多项目待办事项工具。
func NewTodoTool(workspace string) *TodoTool {
	return &TodoTool{
		BaseTool: NewBaseTool(
			"todo",
			"待办事项管理工具（Agentic Task Manager）- 支持多项目/多任务的规划、执行和跟踪。\n\n设计理念：\n- Plan（规划）：使用 add_todos 自动创建项目并写入任务计划\n- Execute（执行）：逐步执行每个待办项\n- Track（跟踪）：通过状态跟踪任务进度\n- Review（回顾）：完成后更新状态并归档项目\n\n可用操作：\n- list_projects: 列出项目\n- add_todos: 批量添加待办事项（按 project_name 自动查找或创建活跃项目）\n- list_todos: 列出项目中的待办\n- update_todo: 更新待办状态\n- archive_project: 归档项目\n- delete_project: 删除项目\n- delete_todo: 删除待办\n\n使用方式：\n1. 规划任务时调用 add_todos，并传入 project_name 与 todos 数组\n2. add_todos 的返回值包含 project_id 和 todo_ids，后续用它们更新状态\n3. 所有步骤完成后调用 archive_project 保持列表整洁",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"operation": map[string]interface{}{
						"type":        "string",
						"enum":        validTodoOperations,
						"description": "操作类型",
					},
					"project_name": map[string]interface{}{
						"type":        "string",
						"description": "项目名称（add_todos 时必需，会自动查找或创建活跃项目）",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "项目描述（add_todos 自动创建项目时使用，可选）",
					},
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "项目ID（list_todos/update_todo/delete_todo/archive_project/delete_project 时必需）",
					},
					"todos": map[string]interface{}{
						"type":        "array",
						"description": "待办事项列表（add_todos 时必需）。每个元素包含 content（必需）和 priority（可选，默认 medium）",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"content": map[string]interface{}{
									"type":        "string",
									"description": "待办内容",
								},
								"priority": map[string]interface{}{
									"type":        "string",
									"enum":        []string{todoPriorityHigh, todoPriorityMedium, todoPriorityLow},
									"description": "优先级",
								},
							},
							"required": []string{"content"},
						},
					},
					"todo_id": map[string]interface{}{
						"type":        "string",
						"description": "待办ID（update_todo/delete_todo 时必需）",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{todoStatusPending, todoStatusInProgress, todoStatusCompleted, todoStatusFailed},
						"description": "状态",
					},
					"include_archived": map[string]interface{}{
						"type":        "boolean",
						"description": "是否包含已归档项目，默认 false",
					},
				},
				"required": []string{"operation"},
			},
		),
		workspace: workspace,
	}
}

func (t *TodoTool) getDirs() (string, string, string) {
	baseDir := filepath.Join(t.workspace, "todos")
	currentDir := filepath.Join(baseDir, "current")
	archiveDir := filepath.Join(baseDir, "archive")
	return baseDir, currentDir, archiveDir
}

func (t *TodoTool) getManifestPath() string {
	baseDir, _, _ := t.getDirs()
	return filepath.Join(baseDir, "manifest.json")
}

func (t *TodoTool) getProjectFilePath(projectID string, archived bool) string {
	_, currentDir, archiveDir := t.getDirs()
	if archived {
		return filepath.Join(archiveDir, projectID+".json")
	}
	return filepath.Join(currentDir, projectID+".json")
}

func (t *TodoTool) ensureDirs() error {
	baseDir, currentDir, archiveDir := t.getDirs()
	for _, dir := range []string{baseDir, currentDir, archiveDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

func (t *TodoTool) loadManifest() (*ManifestData, error) {
	if err := t.ensureDirs(); err != nil {
		return nil, err
	}

	manifestPath := t.getManifestPath()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newManifest(), nil
		}
		return nil, fmt.Errorf("读取项目索引失败: %w", err)
	}

	var manifest ManifestData
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析项目索引失败: %w", err)
	}
	normalizeManifest(&manifest)
	return &manifest, nil
}

func (t *TodoTool) saveManifest(manifest *ManifestData) error {
	if manifest == nil {
		return errors.New("项目索引不能为空")
	}
	if err := t.ensureDirs(); err != nil {
		return err
	}

	normalizeManifest(manifest)
	manifest.UpdatedAt = time.Now()
	return atomicWriteJSON(t.getManifestPath(), manifest)
}

func (t *TodoTool) loadProjectTodos(projectID string) (*TodoListData, error) {
	currentPath := t.getProjectFilePath(projectID, false)
	todoData, err := readTodoFile(currentPath, projectID)
	if err == nil {
		return todoData, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	archivePath := t.getProjectFilePath(projectID, true)
	todoData, err = readTodoFile(archivePath, projectID)
	if err == nil {
		return todoData, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &TodoListData{ProjectID: projectID, Todos: []TodoItem{}}, nil
	}
	return nil, err
}

func (t *TodoTool) saveProjectTodos(data *TodoListData) error {
	if data == nil {
		return errors.New("待办数据不能为空")
	}
	if err := t.ensureDirs(); err != nil {
		return err
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return err
	}

	archived := false
	if idx := projectIndexByID(manifest, data.ProjectID); idx >= 0 {
		archived = manifest.Projects[idx].Status == projectStatusArchived
	}

	data.Timestamp = time.Now()
	if data.Todos == nil {
		data.Todos = []TodoItem{}
	}
	return atomicWriteJSON(t.getProjectFilePath(data.ProjectID, archived), data)
}

func readTodoFile(path string, projectID string) (*TodoListData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var todoData TodoListData
	if err := json.Unmarshal(data, &todoData); err != nil {
		return nil, fmt.Errorf("解析待办文件 %s 失败: %w", filepath.Base(path), err)
	}
	normalizeTodoData(&todoData, projectID)
	return &todoData, nil
}

func atomicWriteJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0644)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err = tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err = tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}

func newManifest() *ManifestData {
	return &ManifestData{
		Version:   todoDataVersion,
		UpdatedAt: time.Now(),
		Projects:  []Project{},
	}
}

func normalizeManifest(manifest *ManifestData) {
	if manifest.Version == "" {
		manifest.Version = todoDataVersion
	}
	if manifest.Projects == nil {
		manifest.Projects = []Project{}
	}
}

func normalizeTodoData(todoData *TodoListData, projectID string) {
	if todoData.ProjectID == "" {
		todoData.ProjectID = projectID
	}
	if todoData.Todos == nil {
		todoData.Todos = []TodoItem{}
	}
}

// Execute 执行待办事项操作。
func (t *TodoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if err := ctx.Err(); err != nil {
		return todoError("操作已取消: " + err.Error()), nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.ensureDirs(); err != nil {
		return todoError("创建目录失败: " + err.Error()), nil
	}

	operation := stringParam(params, "operation")
	switch operation {
	case "":
		return todoError("operation 是必需参数"), nil
	case todoOperationListProjects:
		return t.handleListProjects(params)
	case todoOperationAddTodos:
		return t.handleAddTodos(params)
	case todoOperationListTodos:
		return t.handleListTodos(params)
	case todoOperationUpdateTodo:
		return t.handleUpdateTodo(params)
	case todoOperationArchiveProject:
		return t.handleArchiveProject(params)
	case todoOperationDeleteProject:
		return t.handleDeleteProject(params)
	case todoOperationDeleteTodo:
		return t.handleDeleteTodo(params)
	default:
		return todoError(fmt.Sprintf("未知操作: %s，有效操作: %s", operation, strings.Join(validTodoOperations, ", "))), nil
	}
}

func (t *TodoTool) handleListProjects(params map[string]interface{}) (string, error) {
	includeArchived, _ := params["include_archived"].(bool)

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}

	activeProjects := make([]Project, 0)
	archivedProjects := make([]Project, 0)
	for _, project := range manifest.Projects {
		switch project.Status {
		case projectStatusArchived:
			archivedProjects = append(archivedProjects, project)
		case projectStatusDeleted:
			continue
		default:
			activeProjects = append(activeProjects, project)
		}
	}

	if len(activeProjects) == 0 && (!includeArchived || len(archivedProjects) == 0) {
		return "# 项目列表\n\n暂无活跃项目。请使用 add_todos 自动创建项目并添加待办。", nil
	}

	var builder strings.Builder
	builder.WriteString("# 项目列表\n\n")
	if len(activeProjects) > 0 {
		builder.WriteString("## 🔵 进行中\n\n")
		for _, project := range activeProjects {
			writeProjectLine(&builder, project)
		}
		builder.WriteString("\n")
	}

	if includeArchived && len(archivedProjects) > 0 {
		builder.WriteString("## 📦 已归档\n\n")
		for _, project := range archivedProjects {
			writeProjectLine(&builder, project)
		}
		builder.WriteString("\n")
	}

	builder.WriteString(fmt.Sprintf("---\n**统计**: 活跃 %d | 归档 %d", len(activeProjects), len(archivedProjects)))
	return builder.String(), nil
}

func (t *TodoTool) handleAddTodos(params map[string]interface{}) (string, error) {
	projectName := stringParam(params, "project_name")
	if projectName == "" {
		return todoError("project_name 是必需参数，用于自动查找或创建项目"), nil
	}

	todoInputs, skippedCount, err := parseTodoInputs(params["todos"])
	if err != nil {
		return todoError(err.Error()), nil
	}
	if len(todoInputs) == 0 {
		return todoError("没有有效的待办项被添加，请检查 todos 参数格式"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}

	now := time.Now()
	projectCreated := false
	projectIndex := activeProjectIndexByName(manifest, projectName)
	if projectIndex < 0 {
		projectCreated = true
		project := Project{
			ID:          uuid.NewString(),
			Name:        projectName,
			Description: stringParam(params, "description"),
			Status:      projectStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
			Stats:       ProjectStats{},
		}
		manifest.Projects = append(manifest.Projects, project)
		projectIndex = len(manifest.Projects) - 1
	}

	project := manifest.Projects[projectIndex]
	todoData, err := t.loadProjectTodos(project.ID)
	if err != nil {
		return todoError("加载待办失败: " + err.Error()), nil
	}
	todoData.ProjectID = project.ID
	todoData.ProjectName = project.Name

	addedTodos := make([]TodoItem, 0, len(todoInputs))
	addedTodoIDs := make([]string, 0, len(todoInputs))
	for _, input := range todoInputs {
		todo := TodoItem{
			ID:        uuid.NewString(),
			Content:   input.Content,
			Status:    todoStatusPending,
			Priority:  normalizePriority(input.Priority),
			CreatedAt: now,
			UpdatedAt: now,
		}
		todoData.Todos = append(todoData.Todos, todo)
		addedTodos = append(addedTodos, todo)
		addedTodoIDs = append(addedTodoIDs, todo.ID)
	}

	manifest.Projects[projectIndex].Stats = calculateStats(todoData.Todos)
	manifest.Projects[projectIndex].UpdatedAt = now

	if err := t.saveProjectTodos(todoData); err != nil {
		return todoError("保存待办失败: " + err.Error()), nil
	}
	if err := t.saveManifest(manifest); err != nil {
		return todoError("保存索引失败: " + err.Error()), nil
	}

	result := map[string]interface{}{
		"status":          "batch_added",
		"project_id":      project.ID,
		"project_name":    project.Name,
		"project_created": projectCreated,
		"count":           len(addedTodos),
		"skipped_count":   skippedCount,
		"todo_ids":        addedTodoIDs,
		"added_todos":     addedTodos,
	}
	if projectCreated {
		result["message"] = fmt.Sprintf("已自动创建项目 %q 并添加 %d 个待办", project.Name, len(addedTodos))
	}
	return JSONString(result), nil
}

func (t *TodoTool) handleListTodos(params map[string]interface{}) (string, error) {
	projectID := stringParam(params, "project_id")
	if projectID == "" {
		return todoError("project_id 是列出待办的必需参数"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}

	idx := projectIndexByID(manifest, projectID)
	if idx < 0 || manifest.Projects[idx].Status == projectStatusDeleted {
		return todoError("未找到项目: " + projectID), nil
	}
	project := manifest.Projects[idx]

	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return todoError("加载待办失败: " + err.Error()), nil
	}
	if len(todoData.Todos) == 0 {
		return fmt.Sprintf("# 项目待办: %s\n\n项目ID: `%s`\n\n暂无待办事项", project.Name, project.ID), nil
	}

	var pending, inProgress, completed, failed []TodoItem
	for _, todo := range todoData.Todos {
		switch todo.Status {
		case todoStatusInProgress:
			inProgress = append(inProgress, todo)
		case todoStatusCompleted:
			completed = append(completed, todo)
		case todoStatusFailed:
			failed = append(failed, todo)
		default:
			pending = append(pending, todo)
		}
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# 项目待办: %s\n\n项目ID: `%s`\n\n", project.Name, project.ID))
	writeTodoSection(&builder, "## 🔄 进行中", inProgress)
	writeTodoSection(&builder, "## ⏳ 待处理", pending)
	writeTodoSection(&builder, "## ✅ 已完成", completed)
	writeTodoSection(&builder, "## ❌ 失败", failed)
	builder.WriteString(fmt.Sprintf("---\n**统计**: 总计 %d | 进行中 %d | 待处理 %d | 已完成 %d | 失败 %d",
		len(todoData.Todos), len(inProgress), len(pending), len(completed), len(failed)))
	return builder.String(), nil
}

func (t *TodoTool) handleUpdateTodo(params map[string]interface{}) (string, error) {
	projectID := stringParam(params, "project_id")
	if projectID == "" {
		return todoError("project_id 是更新待办的必需参数"), nil
	}
	todoID := stringParam(params, "todo_id")
	if todoID == "" {
		return todoError("todo_id 是更新待办的必需参数"), nil
	}
	status := stringParam(params, "status")
	if !isValidTodoStatus(status) {
		return todoError("无效的状态，请使用: pending, in_progress, completed, failed"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}
	projectIndex := projectIndexByID(manifest, projectID)
	if projectIndex < 0 || manifest.Projects[projectIndex].Status == projectStatusDeleted {
		return todoError("未找到项目: " + projectID), nil
	}

	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return todoError("加载待办失败: " + err.Error()), nil
	}

	now := time.Now()
	todoIndex := todoIndexByID(todoData, todoID)
	if todoIndex < 0 {
		return todoError("未找到待办: " + todoID), nil
	}

	todoData.Todos[todoIndex].Status = status
	todoData.Todos[todoIndex].UpdatedAt = now
	if status == todoStatusCompleted {
		todoData.Todos[todoIndex].CompletedAt = now
	} else {
		todoData.Todos[todoIndex].CompletedAt = time.Time{}
	}

	manifest.Projects[projectIndex].Stats = calculateStats(todoData.Todos)
	manifest.Projects[projectIndex].UpdatedAt = now

	if err := t.saveProjectTodos(todoData); err != nil {
		return todoError("保存待办失败: " + err.Error()), nil
	}
	if err := t.saveManifest(manifest); err != nil {
		return todoError("保存索引失败: " + err.Error()), nil
	}

	return JSONString(map[string]interface{}{
		"status":     "updated",
		"todo_id":    todoID,
		"project_id": projectID,
		"new_status": status,
	}), nil
}

func (t *TodoTool) handleArchiveProject(params map[string]interface{}) (string, error) {
	projectID := stringParam(params, "project_id")
	if projectID == "" {
		return todoError("project_id 是归档项目的必需参数"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}
	projectIndex := projectIndexByID(manifest, projectID)
	if projectIndex < 0 || manifest.Projects[projectIndex].Status == projectStatusDeleted {
		return todoError("未找到项目: " + projectID), nil
	}
	if manifest.Projects[projectIndex].Status == projectStatusArchived {
		return todoError("项目已归档: " + projectID), nil
	}

	if err := t.moveProjectFileToArchive(projectID); err != nil {
		return todoError("移动待办文件到归档目录失败: " + err.Error()), nil
	}

	now := time.Now()
	manifest.Projects[projectIndex].Status = projectStatusArchived
	manifest.Projects[projectIndex].UpdatedAt = now
	if err := t.saveManifest(manifest); err != nil {
		return todoError("保存索引失败: " + err.Error()), nil
	}

	return JSONString(map[string]interface{}{
		"status":       projectStatusArchived,
		"project_id":   projectID,
		"project_name": manifest.Projects[projectIndex].Name,
	}), nil
}

func (t *TodoTool) handleDeleteProject(params map[string]interface{}) (string, error) {
	projectID := stringParam(params, "project_id")
	if projectID == "" {
		return todoError("project_id 是删除项目的必需参数"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}
	projectIndex := projectIndexByID(manifest, projectID)
	if projectIndex < 0 || manifest.Projects[projectIndex].Status == projectStatusDeleted {
		return todoError("未找到项目: " + projectID), nil
	}

	now := time.Now()
	projectName := manifest.Projects[projectIndex].Name
	manifest.Projects[projectIndex].Status = projectStatusDeleted
	manifest.Projects[projectIndex].UpdatedAt = now
	if err := t.saveManifest(manifest); err != nil {
		return todoError("保存索引失败: " + err.Error()), nil
	}

	if err := t.removeProjectFiles(projectID); err != nil {
		return todoError("删除待办文件失败: " + err.Error()), nil
	}

	return JSONString(map[string]interface{}{
		"status":       projectStatusDeleted,
		"project_id":   projectID,
		"project_name": projectName,
	}), nil
}

func (t *TodoTool) handleDeleteTodo(params map[string]interface{}) (string, error) {
	projectID := stringParam(params, "project_id")
	if projectID == "" {
		return todoError("project_id 是删除待办的必需参数"), nil
	}
	todoID := stringParam(params, "todo_id")
	if todoID == "" {
		return todoError("todo_id 是删除待办的必需参数"), nil
	}

	manifest, err := t.loadManifest()
	if err != nil {
		return todoError("加载索引失败: " + err.Error()), nil
	}
	projectIndex := projectIndexByID(manifest, projectID)
	if projectIndex < 0 || manifest.Projects[projectIndex].Status == projectStatusDeleted {
		return todoError("未找到项目: " + projectID), nil
	}

	todoData, err := t.loadProjectTodos(projectID)
	if err != nil {
		return todoError("加载待办失败: " + err.Error()), nil
	}

	todoIndex := todoIndexByID(todoData, todoID)
	if todoIndex < 0 {
		return todoError("未找到待办: " + todoID), nil
	}

	todoData.Todos = append(todoData.Todos[:todoIndex], todoData.Todos[todoIndex+1:]...)
	now := time.Now()
	manifest.Projects[projectIndex].Stats = calculateStats(todoData.Todos)
	manifest.Projects[projectIndex].UpdatedAt = now

	if err := t.saveProjectTodos(todoData); err != nil {
		return todoError("保存待办失败: " + err.Error()), nil
	}
	if err := t.saveManifest(manifest); err != nil {
		return todoError("保存索引失败: " + err.Error()), nil
	}

	return JSONString(map[string]interface{}{
		"status":     projectStatusDeleted,
		"todo_id":    todoID,
		"project_id": projectID,
	}), nil
}

func (t *TodoTool) moveProjectFileToArchive(projectID string) error {
	currentPath := t.getProjectFilePath(projectID, false)
	archivePath := t.getProjectFilePath(projectID, true)
	if err := os.Rename(currentPath, archivePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func (t *TodoTool) removeProjectFiles(projectID string) error {
	for _, archived := range []bool{false, true} {
		path := t.getProjectFilePath(projectID, archived)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

type todoInput struct {
	Content  string
	Priority string
}

func parseTodoInputs(value interface{}) ([]todoInput, int, error) {
	if value == nil {
		return nil, 0, errors.New("todos 是批量添加待办的必需参数，应为数组格式")
	}

	var items []interface{}
	switch typed := value.(type) {
	case []interface{}:
		items = typed
	case []map[string]interface{}:
		items = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	case []map[string]string:
		items = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			converted := make(map[string]interface{}, len(item))
			for key, val := range item {
				converted[key] = val
			}
			items = append(items, converted)
		}
	default:
		return nil, 0, errors.New("todos 参数格式错误，应为数组格式")
	}

	if len(items) == 0 {
		return nil, 0, errors.New("todos 列表不能为空")
	}

	inputs := make([]todoInput, 0, len(items))
	skipped := 0
	for _, item := range items {
		todoMap, ok := item.(map[string]interface{})
		if !ok {
			skipped++
			continue
		}
		content := strings.TrimSpace(fmt.Sprint(todoMap["content"]))
		if content == "" || content == "<nil>" {
			skipped++
			continue
		}
		priority, _ := todoMap["priority"].(string)
		inputs = append(inputs, todoInput{Content: content, Priority: priority})
	}

	return inputs, skipped, nil
}

func calculateStats(todos []TodoItem) ProjectStats {
	stats := ProjectStats{Total: len(todos)}
	for _, todo := range todos {
		switch todo.Status {
		case todoStatusCompleted:
			stats.Completed++
		case todoStatusInProgress:
			stats.InProgress++
		case todoStatusFailed:
			stats.Failed++
		default:
			stats.Pending++
		}
	}
	return stats
}

func projectIndexByID(manifest *ManifestData, projectID string) int {
	if manifest == nil {
		return -1
	}
	for i := range manifest.Projects {
		if manifest.Projects[i].ID == projectID {
			return i
		}
	}
	return -1
}

func activeProjectIndexByName(manifest *ManifestData, projectName string) int {
	if manifest == nil {
		return -1
	}
	for i := range manifest.Projects {
		if manifest.Projects[i].Name == projectName && manifest.Projects[i].Status == projectStatusActive {
			return i
		}
	}
	return -1
}

func todoIndexByID(todoData *TodoListData, todoID string) int {
	if todoData == nil {
		return -1
	}
	for i := range todoData.Todos {
		if todoData.Todos[i].ID == todoID {
			return i
		}
	}
	return -1
}

func isValidTodoStatus(status string) bool {
	_, ok := validTodoStatuses[status]
	return ok
}

func normalizePriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case todoPriorityHigh:
		return todoPriorityHigh
	case todoPriorityLow:
		return todoPriorityLow
	default:
		return todoPriorityMedium
	}
}

func stringParam(params map[string]interface{}, name string) string {
	value, _ := params[name].(string)
	return strings.TrimSpace(value)
}

func todoError(message string) string {
	return JSONString(map[string]interface{}{"error": message})
}

func writeProjectLine(builder *strings.Builder, project Project) {
	progress := 0
	if project.Stats.Total > 0 {
		progress = (project.Stats.Completed * 100) / project.Stats.Total
	}
	builder.WriteString(fmt.Sprintf("- **%s** (`%s`) - 状态: `%s`, 进度: %d%% (%d/%d)\n",
		project.Name, project.ID, project.Status, progress, project.Stats.Completed, project.Stats.Total))
	if project.Description != "" {
		builder.WriteString(fmt.Sprintf("  - %s\n", project.Description))
	}
}

func writeTodoSection(builder *strings.Builder, title string, todos []TodoItem) {
	if len(todos) == 0 {
		return
	}

	builder.WriteString(title)
	builder.WriteString("\n")
	for _, todo := range todos {
		content := todo.Content
		if todo.Status == todoStatusCompleted {
			content = "~~" + content + "~~"
		}
		builder.WriteString(fmt.Sprintf("- %s `%s` **[%s]** %s\n",
			priorityIcon(todo.Priority), todo.ID, todo.Status, content))
	}
	builder.WriteString("\n")
}

func priorityIcon(priority string) string {
	switch priority {
	case todoPriorityHigh:
		return "🔴"
	case todoPriorityLow:
		return "🟢"
	default:
		return "📌"
	}
}
