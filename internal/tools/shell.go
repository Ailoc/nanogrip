package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// shell.go - Shell命令执行工具
// 此文件实现了在系统shell中执行命令的工具，支持超时控制

// ShellTool 提供shell命令执行功能
// 允许代理执行系统命令并获取输出结果，支持bash和sh
// 注意：对于需要交互式输入的命令，请使用 tmux 技能
type ShellTool struct {
	BaseTool
	timeout time.Duration // 命令执行超时时间
}

// NewShellTool 创建一个新的shell工具
// 参数:
//
//	timeout: 命令执行超时时间（秒）
//
// 返回:
//
//	配置好的ShellTool实例
func NewShellTool(timeout int) *ShellTool {
	return &ShellTool{
		BaseTool: NewBaseTool(
			"shell",
			"Execute a shell command and return its output. For interactive commands (passwords, confirmations), use the tmux skill.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The shell command to execute",
					},
				},
				"required": []string{"command"},
			},
		),
		timeout: time.Duration(timeout) * time.Second,
	}
}

// Execute 执行shell命令
// 在系统shell中执行指定命令，返回标准输出和标准错误
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"command"字段
//
// 返回:
//
//	命令的标准输出内容，如果有错误则附加stderr信息
func (t *ShellTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取命令参数
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing or invalid command parameter")
	}

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 确定使用的shell
	shell := "/bin/sh"
	args := []string{"-c", command}

	// 检查是否有shebang指定使用bash
	if strings.HasPrefix(command, "#!") {
		parts := strings.SplitN(command, "\n", 2)
		if len(parts) > 0 {
			shebang := strings.TrimPrefix(parts[0], "#!")
			if shebang == "/bin/bash" || shebang == "/usr/bin/env bash" {
				shell = "/bin/bash"
			}
		}
	}

	// 创建命令
	cmd := exec.CommandContext(timeoutCtx, shell, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置 stdin 为 nil，防止命令等待交互式输入
	cmd.Stdin = nil

	// 执行命令
	err := cmd.Run()
	output := stdout.String()

	// 处理错误情况
	if err != nil {
		// 检查是否超时
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", t.timeout)
		}

		// 检查是否是退出码错误
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			// 如果命令失败（退出码非0），返回有意义的错误信息
			if stderr.Len() > 0 {
				return fmt.Sprintf("[exit code %d] %s", exitCode, stderr.String()), nil
			}
			// 如果没有stderr但命令失败，返回退出码信息
			if output == "" {
				return fmt.Sprintf("[exit code %d] command failed with no output", exitCode), nil
			}
			return fmt.Sprintf("[exit code %d] %s", exitCode, output), nil
		}

		// 其他错误
		if stderr.Len() > 0 {
			return output + "\n[stderr]: " + stderr.String(), err
		}
		return output, err
	}

	// 命令成功执行，但可能没有输出
	// 确保不为 LLM 返回空内容
	if output == "" {
		return "(command completed successfully with no output)", nil
	}

	return output, nil
}
