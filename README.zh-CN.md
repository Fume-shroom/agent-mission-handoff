# Agent Mission Handoff

[English](README.md) | **简体中文**

通过一个便携的 `.amh` 文件，在不同机器、团队成员、Codex 与 Claude Code 之间迁移并继续执行 AI Coding Mission。

<p align="center">
  <img src="docs/assets/amh-demo.gif" alt="AMH 发送端与接收端工作流演示" width="100%">
</p>

### 通过 Coding Agent 使用

| 发送方 | 接收方 |
| --- | --- |
| 告诉 Agent：**“把当前任务交接成一个 AMH 文件。”** | 把 `mission.amh` 交给 Agent，然后说：**“继续这个任务。”** |
| Agent 会运行 `amh pack`，生成一个便携的 `mission.amh` 文件。 | Agent 会运行 `amh continue mission.amh`，总结恢复出的上下文，然后询问是否继续。 |

也可以直接使用命令行：

```bash
# 发送端
amh pack

# 接收端
amh continue mission.amh
```

AMH 是本地优先工具：会话交接本身不需要守护进程、云服务、账号、数据库或 GitHub 仓库。

> 技术预览：Codex 与 Claude Code 的会话格式属于私有实现细节，产品升级后可能需要更新 Adapter。

## 安装

macOS 或 Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1 | iex
```

用户不需要安装 Go，也不需要下载源码。安装器会验证 Release 校验和，并同时安装 CLI、Codex Skill 和 Claude Code Skill。

你也可以直接告诉本地 Coding Agent：

> 从 https://github.com/Fume-shroom/agent-mission-handoff 安装 AMH，并验证安装结果。

## Agent 对话或命令行

AMH 的每个操作都支持交给具备本地操作能力的 Coding Agent 完成，或者直接运行命令。

| 步骤 | 告诉 Coding Agent | 命令行 |
| --- | --- | --- |
| 安装 | “从这个仓库安装 AMH，并验证安装结果。” | 运行上面的安装命令 |
| 打包 | “把当前任务交接成一个 AMH 文件。” | `amh pack` |
| 查看 | “查看这个交接文件，并总结它包含的内容。” | `amh inspect mission.amh` |
| 恢复 | 把文件交给 Agent，然后说：“继续这个任务。” | `amh continue mission.amh` |

Agent 对话模式内部仍然调用本地 `amh` CLI，不依赖 AMH 云服务。

## 接收端体验

`amh continue` 会恢复一个可继续操作的 Session，并自动输出简洁的 Mission Brief，包括：

- 原始目标与当前状态；
- 已恢复的历史 turn 数和最近的对话上下文；
- 已完成工作、未解决问题和建议的下一步；
- 缺失的 Skills、MCP、CLI 或工作区条件；
- 原生 Session 恢复命令。

当接收方是 Coding Agent 时，它还会读取完整的可迁移 transcript，并在第一次回复中输出质量更高的 Mission Brief，然后主动询问是否继续。在用户确认之前，它不能运行任务工具或修改项目文件。

完整历史仍保留在恢复后的可写 Session 中。默认只输出摘要和关键上下文，不会把整段 transcript 全量倾倒到聊天界面。

## 文件包含什么

一个 `.amh` 文件包含：

- Mission Checkpoint：目标、进度、已完成工作、风险和下一步；
- 可迁移对话历史与原始 Agent Session；
- 工作区与 Git 身份信息；
- 实际使用过的 Skills、MCP 和 CLI；
- 校验和与压缩包安全元数据。

AMH **不会**传输凭证、权限授权、登录状态、运行中的进程、模型私有状态或项目代码仓库。

## 支持的迁移方向

| 发送端 | 接收端 | 恢复方式 |
| --- | --- | --- |
| Codex | Codex | 原生可写 Fork |
| Claude Code | Claude Code | 原生可写 Fork |
| Codex | Claude Code | 语义会话转换 |
| Claude Code | Codex | 语义会话转换 |

跨 Agent 恢复会保留有价值的对话和任务上下文，但不会声称逐字节复刻私有运行状态或工具调用内部信息。

## 文档

- [用户手册与 CLI 参考](docs/USER_GUIDE.md)
- [使用教程](docs/tutorials/README.md)
- [架构与 Capsule 格式](docs/ARCHITECTURE.md)
- [安全策略](SECURITY.md)
- [参与贡献](CONTRIBUTING.md)

## License

MIT。第三方声明见 [NOTICE](NOTICE)。
