package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// filesystem.go - 文件系统操作工具
// 此文件实现了文件和目录的读取、写入、列表、删除和检查功能
// 支持工作区限制以提高安全性

// FilesystemTool 提供文件操作功能
// 允许代理读取、写入、列出、删除文件和目录，可选择限制在工作区内
type FilesystemTool struct {
	BaseTool
	workspace string // 工作区目录路径
	restrict  bool   // 是否限制操作仅在工作区内
}

// NewFilesystemTool 创建一个新的文件系统工具
// 参数:
//
//	workspace: 工作区目录路径
//	restrict: 是否限制操作仅在工作区内（true为限制，false为允许访问任意路径）
//
// 返回:
//
//	配置好的FilesystemTool实例
func NewFilesystemTool(workspace string, restrict bool) *FilesystemTool {
	return &FilesystemTool{
		BaseTool: NewBaseTool(
			"filesystem",
			"Perform file operations (read, write, list, delete)",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"operation": map[string]interface{}{
						"type":        "string",
						"description": "Operation to perform: read, write, list, delete, exists",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "File or directory path",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write (for write operation)",
					},
				},
				"required": []string{"operation", "path"},
			},
		),
		workspace: workspace,
		restrict:  restrict,
	}
}

// Execute 执行文件系统操作
// 根据operation参数执行相应的文件操作
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"operation"和"path"，写入操作需要"content"
//
// 返回:
//
//	操作结果字符串（读取、列表、检查操作）或空字符串（写入、删除操作）
func (t *FilesystemTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取操作类型
	operation, ok := params["operation"].(string)
	if !ok {
		return "", fmt.Errorf("missing operation parameter")
	}

	// 获取路径参数
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing path parameter")
	}

	// 解析路径（处理相对路径和工作区限制）
	resolvedPath, err := t.resolvePath(path)
	if err != nil {
		return "", err
	}

	// 根据操作类型执行相应功能
	switch operation {
	case "read":
		return t.readFile(resolvedPath)
	case "write":
		content, _ := params["content"].(string)
		err := t.writeFile(resolvedPath, content)
		if err != nil {
			return "", err
		}
		// 【关键修复】返回有意义的消息，而不是空字符串
		// 这确保 LLM 知道操作成功，可以继续执行下一步
		return fmt.Sprintf("File written successfully: %s (%d bytes)", resolvedPath, len(content)), nil
	case "list":
		return t.listDir(resolvedPath)
	case "delete":
		err := t.deletePath(resolvedPath)
		if err != nil {
			return "", err
		}
		// 【关键修复】返回有意义的消息
		return fmt.Sprintf("File deleted successfully: %s", resolvedPath), nil
	case "exists":
		return fmt.Sprintf("%v", t.exists(resolvedPath)), nil
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

// resolvePath 解析路径，处理相对路径和工作区限制
// 将相对路径转换为绝对路径，并检查是否在工作区内（如果启用了限制）
// 参数:
//
//	path: 原始路径（可以是相对路径、绝对路径或~开头的路径）
//
// 返回:
//
//	解析后的绝对路径，如果违反限制则返回错误
func (t *FilesystemTool) resolvePath(path string) (string, error) {
	// 展开用户主目录
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	} else if !filepath.IsAbs(path) {
		// 相对路径 - 使用工作区
		path = filepath.Join(t.workspace, path)
	}

	// 解析为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// 检查工作区限制
	if t.restrict {
		absWorkspace, err := filepath.Abs(t.workspace)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(absPath, absWorkspace) {
			return "", fmt.Errorf("path '%s' is outside workspace", path)
		}
	}

	return absPath, nil
}

// readFile 读取文件内容
// 参数:
//
//	path: 文件的绝对路径
//
// 返回:
//
//	文件内容字符串
func (t *FilesystemTool) readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeFile 写入内容到文件
// 如果文件所在目录不存在，会自动创建目录
// 参数:
//
//	path: 文件的绝对路径
//	content: 要写入的内容
//
// 返回:
//
//	如果写入失败则返回错误
func (t *FilesystemTool) writeFile(path string, content string) error {
	dir := filepath.Dir(path)
	// 创建所需的目录（如果不存在）
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// listDir 列出目录内容
// 返回目录中所有文件和子目录的名称，目录名后带"/"，文件名后带大小信息
// 参数:
//
//	path: 目录的绝对路径
//
// 返回:
//
//	格式化的目录列表字符串
func (t *FilesystemTool) listDir(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var result []string
	for _, e := range entries {
		info, _ := e.Info()
		if e.IsDir() {
			result = append(result, fmt.Sprintf("%s/", e.Name()))
		} else {
			result = append(result, fmt.Sprintf("%s (%d bytes)", e.Name(), info.Size()))
		}
	}

	if len(result) == 0 {
		return "(empty directory)", nil
	}

	return strings.Join(result, "\n"), nil
}

// deletePath 删除文件或目录
// 如果是目录则递归删除所有内容
// 参数:
//
//	path: 要删除的文件或目录的绝对路径
//
// 返回:
//
//	如果删除失败则返回错误
func (t *FilesystemTool) deletePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		// 递归删除目录及其内容
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// exists 检查路径是否存在
// 参数:
//
//	path: 要检查的路径
//
// 返回:
//
//	如果路径存在返回true，否则返回false
func (t *FilesystemTool) exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
