# ccecho

  为 Claude Code / Codex CLI 提供透明记录、调用可视化与本地回放的轻量 CLI。

## 产品介绍

`ccecho` 是一个面向 `Claude Code` 和 `Codex CLI` 的轻量级 CLI 工具。它运行在你的 agent 与上游模型服务之间，自动记录每一轮请求、响应和会话元信息，并提供一个本地 Web Viewer 供你回放、排查和复盘。

它上手快、体积小、接入轻，用一个本地 CLI 就能低成本为 agent 的API调用 补上记录、可视化与回放能力。

## 立即开始

1. 下载对应平台二进制。

2. 把对应的 `ccecho` 或 `ccecho.exe` 加入环境变量。

3. 验证命令可用：

```bash
ccecho --help
```

4. 运行 agent 并自动接入代理：

```bash
ccecho run [claude|codex]
```

5. 启动本地查看器：

```bash
ccecho view
```

默认地址：http://127.0.0.1:18080

## 从源码构建

如果你希望自己编译当前平台版本：

```bash
go build -o ccecho ./cmd/ccecho
```

## ccecho 解决什么问题

- 你想复盘一段高质量对话，却拿不到完整上下文
- 你想比较 Claude 和 Codex 的实际行为，却缺少统一观察入口
- 你想让团队基于“事实记录”沟通，而不是基于个人印象沟通
- 你怀疑是 prompt、上下文或工具调用出了问题，但终端里只剩零散输出
- 你想把一次成功的 agent 使用方式分享给同事，却没有一份可回看的完整记录
- 你想长期积累真实的 AI 开发样本，而不是让每次调用都在结束后直接消失

`ccecho` 会把这些原本稍纵即逝的调用过程沉淀为本地可追溯记录，让排障、复盘、比较和协作都建立在真实请求与响应之上。

## 产品特点

- CLI 工具，0侵入接入现有工作流
- 轻量可执行文件，不需要额外部署一套服务
- 本地运行，本地存储，接入和移除都简单
- 不替换你现有 agent，只补上记录和复盘能力

## 核心能力

- 本地代理 Claude Code / Codex CLI 请求
- 按会话保存请求体、响应体、流式响应和元数据
- 解析 Claude / Codex 的SSE响应块，尽量还原文本、thinking、tool use 等信息
- 支持回放完整调用过程，查看 token 使用情况和原始 JSON
- 支持清理无有效请求/响应内容的空日志目录

## 适用场景

- 排查模型效果不稳定、上下文漂移、工具调用异常
- 通过查看请求体与响应体，学习 agent 在真实任务中的实现方式
- 观察 agent 的工具调用与命令执行过程，及时发现潜在危险操作
- 通过复盘调用方式分析上下文组织与 token 消耗，评估成本变化
- 对比不同 agent 在真实工程任务上的行为差异
- 长期保留本地 AI 开发会话，形成可检索资产

## 适合谁

- 高频使用 Claude Code / Codex CLI 的个人开发者
- 需要定位真实请求与响应问题的工具链或平台工程师
- 想通过真实请求与响应学习 agent 工作方式的学习者
- 需要关注 agent 执行关键命令、工具调用是否符合预期的使用者
- 希望分析 agent 调用方式与 token 消耗、评估使用成本的使用者
- 希望保留本地会话记录，而不是只看终端滚动输出的使用者

## 命令说明

### `ccecho run`

启动本地代理并拉起目标 CLI。

```bash
ccecho run claude
ccecho run codex
```

### `ccecho view`

启动本地 viewer，查看已经保存的会话和请求详情。

```bash
ccecho view
```

### `ccecho clean`

清理没有有效请求或响应内容的空日志目录。

```bash
ccecho clean
```

## 工作方式

`ccecho` 的工作流很简单：

1. 启动本地代理。
2. 用代理地址覆盖 Claude Code 或 Codex CLI 的请求地址。
3. agent 的请求先经过 `ccecho`，再被转发到真实模型服务。
4. `ccecho` 将请求、流式响应、最终响应和会话元信息写入 `.ccecho/`。
5. 通过 `ccecho view` 启动本地 viewer，在浏览器中回看整个过程。

## 实现概览

项目整体分成三层：

- `cmd/ccecho`
  CLI 入口，负责命令解析、启动代理、拉起 Claude / Codex、启动 viewer
- `internal/proxy`
  本地反向代理，把请求转发到上游，并在转发过程中完成日志落盘
- `internal/viewer`
  本地 Web Viewer，提供页面和 JSON API，用于查看会话、请求和响应细节

其余重要模块：

- `internal/logstore`
  按会话目录管理请求/响应文件，异步写盘队列落地
- `internal/requestview`
  针对 Claude / Codex 的请求格式做解析，统一显示
- `internal/stream`
  处理流式响应，尽量还原成最终消息 JSON
- `internal/sessionmeta` 、 `internal/state`
  保存当前会话元信息与最近状态
