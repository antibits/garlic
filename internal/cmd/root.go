package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/antibits/garlic/internal/logger"
)

var (
	cfgPath string
	debug   bool
)

// rootCmd 是 Garlic AI Agent 的根命令
var rootCmd = &cobra.Command{
	Use:   "garlic",
	Short: "Garlic AI Agent - 通用 AI 助手框架",
	Long: `Garlic 是一个通用 AI 助手框架，支持工具执行、多会话管理和多 LLM 提供商。

主要功能:
  - 意图路由：自动分类请求为工具调用或简单查询
  - 工具执行：支持 Go 原生工具和 Python 脚本
  - 工具生成：可根据功能描述自动生成新工具
  - 多 LLM 支持：可配置 OpenAI 和阿里百炼模型
  - 多会话管理：支持多个独立对话会话`,
	// 默认运行 serve 命令
	RunE: func(cmd *cobra.Command, args []string) error {
		// 如果没有指定子命令，默认启动 serve 模式
		return runServe(cmd, args)
	},
}

// Execute 执行根命令
func Execute() error {
	// Add a persistent pre-run function to initialize logger
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if err := logger.Init(debug); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
	}

	return rootCmd.Execute()
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", "配置文件路径")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "启用调试模式")
}
