# Agent Mission Handoff

[English](README.md) | **简体中文**

**把 Codex 或 Claude Code 中正在进行的任务打包成一个 `.amh` 文件，在另一台机器或另一个 Agent 中恢复为可继续工作的 Session。**

Codex ↔ Claude Code · 本地优先 · 不需要云服务、账号、数据库或 GitHub 仓库

## 两句话完成交接

- **发送方：**告诉当前 Agent **“把当前任务交接成一个 AMH 文件。”** → Agent 生成 `mission.amh`（`amh pack`）。
- **接收方：**把 `mission.amh` 交给目标 Agent 并说 **“继续这个任务。”** → Agent 恢复可写 Session，先给出 Mission Brief，再询问是否继续（`amh continue mission.amh`）。

发送方一句话，接收方一句话，中间只传一个文件。

<p align="center">
  <img src="docs/assets/amh-demo.gif" alt="将 Coding Agent 任务打包为 mission.amh，在另一个 Agent 中恢复、查看 Mission Brief 并继续工作" width="820">
</p>

## 只需安装一次

优先直接告诉 Coding Agent：

> 从 https://github.com/Fume-shroom/agent-mission-handoff 安装 AMH，并验证安装结果。

也可以直接运行安装命令。

macOS 或 Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1 | iex
```

安装器会验证 Release 校验和，并安装一个 CLI，以及 Codex 和 Claude Code 共用的 Mission Handoff Skill。用户不需要安装 Go，也不需要下载源码。

需要检查时运行 `amh doctor`。

## 说出“继续这个任务”之后

- AMH 会检查文件完整性、目标工作区、Git 上下文，以及本次继续任务可能需要的 Skills、MCP 和 CLI。
- 如果缺少必要能力或路径映射，Agent 会说明真正相关的差异，并在安装、登录、重新映射或忽略差异前只询问一次。
- AMH 默认使用安全语义恢复创建可写 Session。原始历史会作为上下文保留，但不会被当成目标 Agent 的可信指令。
- 恢复后的 Agent 会先输出 Mission Brief：目标、历史、已完成工作、未解决问题、环境差异和建议的下一步。
- 用户确认前，Agent 不会运行任务工具，也不会应用源端代码补丁。

## 一个文件包含什么

一个 `.amh` 文件可以包含：

- 任务目标、当前进度、已完成工作、风险和下一步；
- 可迁移对话历史与原始 Agent Session；
- 工作区和 Git 身份信息；
- 可选的 tracked、untracked 和 staged 改动补丁；
- 实际使用过的 Skills、MCP 和 CLI，以及可获得的来源、版本和摘要信息；
- 校验和与压缩包安全元数据。

AMH 不会主动复制 Agent 凭据、登录状态、权限授权、运行中的进程、模型私有状态或完整代码仓库。会话和补丁仍可能包含敏感值。AMH 默认执行尽力而为的高置信度脱敏，但发送前仍应检查文件，并使用获批的安全渠道传输。

## 常用命令

| 需要完成的操作 | 命令 |
| --- | --- |
| 打包当前任务 | `amh pack` |
| 恢复并继续 | `amh continue mission.amh` |
| 只查看、不恢复 | `amh inspect mission.amh` |
| 应用已确认的源端改动 | `amh apply mission.amh` |
| 检查本地环境 | `amh doctor` |
| 更新 AMH | `amh update` |
| 卸载 AMH | `amh uninstall` |

## 支持的迁移方向

| 发送端 | 接收端 | 恢复方式 |
| --- | --- | --- |
| Codex | Codex | 默认安全语义恢复为可写 Session |
| Claude Code | Claude Code | 默认安全语义恢复为可写 Session |
| Codex | Claude Code | 语义 Session 转换 |
| Claude Code | Codex | 语义 Session 转换 |

对于完全可信的 Capsule，可以通过 `--trust-native-session` 显式启用同 Agent 原生 Fork。AMH 不会声称逐字节复刻私有运行状态或工具调用内部信息。

> 技术预览：Codex 与 Claude Code 的 Session 格式属于私有实现细节，产品升级后可能需要更新 Adapter。

## 文档

- [用户手册与 CLI 参考](docs/USER_GUIDE.md)
- [使用教程](docs/tutorials/README.md)
- [架构与 Capsule 格式](docs/ARCHITECTURE.md)
- [安全策略](SECURITY.md)
- [参与贡献](CONTRIBUTING.md)

## License

MIT。第三方声明见 [NOTICE](NOTICE)。
