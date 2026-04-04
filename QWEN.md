# Garlic - Project Context

## 项目概述

**Garlic** 是一个通用 AI Agent 框架，类似于 Qwen Code。它具有工具执行和上下文管理能力，支持多个 LLM 提供商。

### 核心能力
- **意图路由**：自动将请求分类为工具调用或简单查询
- **工具执行**：支持 Go 原生工具和 Python 脚本
- **工具生成器**：可根据功能描述自动创建新的 Python 工具
- **多 LLM 支持**：可配置 OpenAI 和阿里巴巴百炼的模型
- **多会话管理**：用户可以维护多个独立的对话会话

### 架构流程
```
用户请求 → Session（添加到对话）
              ↓
    工作流管道（路由 → 执行）
              ↓
   （工具 | 计划 | 简单）
              ↓
  Session（更新对话和待办事项）
```

## 构建和运行

### 环境要求
- Go 1.26+
- Python 3.11+
- LLM 提供商的 API 密钥

### 安装
```bash
# 安装 Go 依赖
go mod tidy
```

### 配置
1. 将 `config.yaml.example` 复制到 `config.yaml`
2. 设置 API 密钥（支持 `${ENV_VAR}` 语法）

### 环境变量
```bash
export OPENAI_API_KEY=your-openai-api-key
export BAILIAN_API_KEY=your-bailian-api-key
```

### 运行
```bash
# 使用默认配置运行
go run cmd/main.go

# 使用自定义配置运行
go run cmd/main.go path/to/config.yaml

# 调试模式运行
go run cmd/main.go -debug
```

### 会话命令
```
/new [name]     - 创建新会话
/list           - 列出所有会话
/switch <id>    - 切换到指定会话
/delete <id>    - 删除会话
/current        - 显示当前会话
```

## 项目结构

```
garlic/
├── cmd/
│   └── main.go                  # 入口点，主 REPL 循环
├── internal/
│   ├── agents/
│   │   ├── agents.go            # Agent 实现（Summarizer, Organizer, ExecutorAgent）
│   │   ├── router.go            # 意图路由器（tool/simple/step_by_step）
│   │   └── agents_test.go       # Agent 测试
│   ├── config/
│   │   └── config.go            # YAML 配置加载
│   ├── harness/
│   │   ├── harness.go           # 核心编排（会话和工作流）
│   │   ├── model/
│   │   │   └── models.go        # 数据模型（Conversation, TodoQueue, ExecutionContext, Stack）
│   │   └── session/
│   │       └── session.go       # 会话和对话管理
│   ├── llm/
│   │   └── client.go            # LLM 客户端封装（OpenAI 兼容）
│   └── tool/
│       ├── discovery.go         # 工具发现和描述生成
│       ├── executor.go          # 工具执行引擎
│       ├── filereader.go        # 内置文件读取工具
│       ├── filewriter.go        # 内置文件写入工具
│       ├── tool_generator.go    # 工具代码生成（创建新 Python 工具）
│       └── tool_generator_tool.go # 作为可调用的工具生成器
├── tools/                       # Python 工具目录
│   └── websearch/               # 工具目录（工具名称）
│       └── main.py              # 网络搜索工具入口点
├── config.yaml                  # 应用配置
├── config.yaml.example          # 配置示例
├── go.mod                       # Go 模块定义
└── QWEN.md                      # 本文件
```

## 核心组件

### Harness (`internal/harness/harness.go`)
主编排器，管理会话和工作流执行：
- `ProcessRequest()`：用户请求入口 - 添加到会话，执行工作流
- 使用 **ExecutionContextStack** 处理嵌套子任务（LIFO）
- 使用 **TodoQueue** 执行多步骤计划（FIFO）
- 适配器将内部 Agent 与工作流接口桥接

### 会话管理 (`internal/harness/session/session.go`)
管理对话历史和执行上下文：
- `Session`：表示单个对话，包含对话历史和执行上下文栈
- `Conversation`：线程安全的消息历史管理
- `Manager`：多会话管理（创建、列表、切换、删除）

### 数据模型 (`internal/harness/model/models.go`)
核心数据结构：
- `Conversation`：管理消息历史，提供实用方法
- `TodoQueue`：管理多步骤计划执行的任务队列（FIFO）
- `ExecutionContext`：保存子任务执行的对话
- `ExecutionContextStack`：用于嵌套子任务处理的 LIFO 栈

### 路由器 (`internal/agents/router.go`)
意图分类，分为三类：
- `tool`：直接工具执行请求
- `simple`：可以直接回答
- `step_by_step`：需要多步骤执行

### Agents (`internal/agents/agents.go`)
专用 AI Agent：
- `SummarizerAgent`：总结对话结果
- `OrganizeAgent`：组织对话内容
- `ExecutorAgent`：确定使用哪个工具
- `Router`：意图分类（tool/simple/step_by_step）

### LLM 客户端 (`internal/llm/client.go`)
统一的 LLM 提供商接口：
- 支持 OpenAI 和百炼（通过 OpenAI 兼容 API）
- 提示模板渲染
- 流式和非流式对话完成
- API 密钥的环境变量扩展

### 工具执行器 (`internal/tool/executor.go`)
处理工具执行：
- 注册的 Go 原生工具
- 来自 `tools/<tool-name>/main.py` 入口点的 Python 脚本
- 参数通过命令行选项传递：`-key value`

### 工具发现 (`internal/tool/discovery.go`)
动态工具发现和描述生成：
- 扫描 `tools/` 目录获取可用的 Python 工具
- 运行 `-h` 获取工具帮助文本
- 缓存工具描述以提高效率
- 注册内置 Go 工具

### 工具生成器 (`internal/tool/tool_generator.go`)
自动创建新的 Python 工具：
- 使用 LLM 根据功能描述生成 Python 代码
- 创建工具目录结构：`tools/<tool-name>/main.py`
- 通过运行 `-h` 验证生成的工具
- 遵循标准的 argparse 模式和 JSON 输出

## 配置格式

### Models 部分
按名称定义 LLM 端点：
```yaml
models:
  openai-gpt4:
    provider: openai
    model: gpt-4
    api_key: ${OPENAI_API_KEY}
    temperature: 0.7
    max_tokens: 2048

  bailian-qwen-coder-plus:
    provider: bailian
    model: qwen-coder-plus
    api_key: ${BAILIAN_API_KEY}
    base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
    enable_thinking: true
```

### Agents 部分
定义引用模型的 Agent 角色：
```yaml
agents:
  router:
    model: qwen3.5-plus-instr
    prompt_template: |
      You are an conversation assistant...

  executor:
    model: qwen3.5-plus-instr

  organizer:
    model: qwen3.5-plus

  summarizer:
    model: qwen3.5-plus
```

### 工具生成器（可选）
启用自动工具创建：
```yaml
tool_generator:
  enabled: true
  model: openai-gpt4
```

### Tools 部分
```yaml
tools:
  python_path: python
  tools_dir: tools
```

## 工作流管道

核心工作流遵循 **路由 → 执行** 模式：

1. **路由**：分类意图（tool/simple/step_by_step）
2. **执行**：
   - **工具**：ExecutorAgent 选择合适的工具 → 执行 → 更新对话
   - **简单**：返回直接响应
   - **分步**：为子任务创建嵌套执行上下文

## 开发规范

### 代码风格
- Go 标准格式化（`gofmt`）
- 带有描述性消息的错误处理
- 用于可取消操作的 Context 传播

### 添加工具

#### Python 工具
Python 工具遵循标准化结构：

1. **目录结构**：在 `tools/` 下创建以工具名称命名的目录
   ```
   tools/
   └── <tool-name>/
       └── main.py
   ```

2. **入口点**：`main.py` 作为工具入口点

3. **命令行接口**：
   - 通过命令行选项接受参数
   - 提供 `-h` / `--help` 选项获取用法帮助
   - 示例：`python tools/websearch/main.py -h`

4. **输入/输出**：
   - 通过命令行选项接收参数（格式：`-key value`）
   - 将结果以 JSON 格式输出到标准输出（stdout）
   - 将错误输出到标准错误（stderr）

5. **示例** - `tools/websearch/main.py`：
   ```python
   #!/usr/bin/env python3
   """Web search tool - search the web for information."""
   
   import argparse
   import json
   
   def main():
       parser = argparse.ArgumentParser(description='Search the web')
       parser.add_argument('-query', type=str, required=True, help='Search query')
       parser.add_argument('-num', type=int, default=5, help='Number of results')
       args = parser.parse_args()
       
       # 执行搜索...
       result = {"success": True, "results": [...]}
       print(json.dumps(result))  # 输出到 stdout
   
   if __name__ == '__main__':
       main()
   ```

#### Go 原生工具
实现 `Tool` 接口：
```go
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
}
```

### 提示模板
- 使用 Go 的 `text/template` 语法
- 通过 `{{.variable}}` 访问变量
- 在 `config.yaml` 中定义或使用代码默认值

## 依赖项

| 包 | 用途 |
|---------|---------|
| `github.com/openai/openai-go` | OpenAI SDK（也用于百炼） |
| `github.com/kaptinlin/jsonrepair` | 修复格式错误的 JSON 响应 |
| `gopkg.in/yaml.v3` | YAML 配置解析 |
| `github.com/gin-gonic/gin` | Web 框架（用于 Web 服务器模式） |
| `github.com/gorilla/websocket` | WebSocket 支持 |
| `github.com/spf13/cobra` | CLI 框架 |
| `go.uber.org/zap` | 日志记录 |
| `github.com/google/uuid` | UUID 生成 |

## 意图分类

路由器将用户请求分为三类：

| 意图 | 描述 | 示例 |
|--------|-------------|---------|
| `tool` | 需要工具执行 | "搜索最新 AI 新闻" |
| `simple` | 可以直接回答 | "2 + 2 等于多少？" |
| `step_by_step` | 需要多步骤执行 | "编写一个爬取网站的脚本" |

## 会话架构

会话支持嵌套执行上下文以处理复杂的多步骤任务：

```
Session
├── Conversation（消息历史）
├── ExecCtxStack（LIFO 用于嵌套子任务）
│   ├── ExecutionContext 1（父）
│   └── ExecutionContext 2（子）
└── TodoQueue（FIFO 用于计划执行）
```

## 消息类型

系统支持两种消息类型以区分用户触发和自动处理的消息：

| 类型 | 描述 |
|------|------|
| `user_triggered` | 用户直接触发的消息 |
| `auto` | 自动处理/思考过程中生成的消息 |

## 会话持久化

会话数据持久化到 `.sessions/` 目录：
- `meta.json`：会话元数据（ID、名称、创建时间、最后活动时间、Token 使用量）
- `messages.jsonl`：JSONL 格式的对话历史

## 对话压缩

当对话超过一定轮数或长度时，会自动触发压缩：
- 默认阈值：20 轮或 2048 字符
- 使用 SummarizerAgent 生成摘要
- 可配置是否启用
