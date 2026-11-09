---
title: 编程智能体
description: 让 Claude Code、Codex 等智能体在身边运行任务，并在 Kero 中汇报进度。
---

# 编程智能体

在 Kero 中运行的智能体可以像你一样使用 `kero` 命令：在下方窗格里启动开发服务器、把评审交给另一个智能体、等待测试跑完，并在受阻时告诉你。

<a id="install-the-skill"></a>
## 安装技能

```sh
kero skill setup
```

这会安装两样东西：

- 一个**技能**，教智能体使用命令行。Claude Code 从 `~/.claude/skills/kero` 读取；Codex、OpenCode 等智能体从 `~/.agents/skills/kero` 读取。
- 一个**钩子**，在智能体开始会话时运行。它告诉智能体自己位于哪个 Kero 会话和窗格，省得它花时间去查；如果会话还没有名字，就让智能体按任务给它命名，例如 `fix-login-redirect`，不用你动手。它还会把智能体自己的会话告诉 Kero，这样 kerod 重启后（比如电脑重启之后），智能体就能[接上之前的对话](./sessions.md#kerod)。智能体每结束一个回合，同一个钩子也会运行，把回合的结束和最后的回复告诉 Kero，这样[编排器](./orchestrators.md)会在子会话的回合结束时得知，而不是等它的屏幕看起来空闲时。此时钩子不输出任何内容，并在后台运行，不会拖慢智能体。

对于你已安装的智能体，钩子会放在各自查找钩子的位置：

| 智能体      | 钩子位置                                              |
| ----------- | ----------------------------------------------------- |
| Claude Code | `~/.claude/settings.json` 中的 `SessionStart`、`Stop` 和 `StopFailure` 钩子，以及 Kero 的插件 `kero@kero` |
| Codex       | `~/.codex/hooks.json` 中的 `SessionStart`、`Stop` 和 `Interrupt` 钩子       |
| Pi          | 扩展 `~/.pi/agent/extensions/kero.ts`                  |
| OMP         | 扩展 `~/.omp/agent/extensions/kero.ts`                 |
| OpenCode    | 插件 `~/.config/opencode/plugins/kero`                 |

Claude Code 的插件位于 Kero 自己的文件夹中，是名为 `kero` 的插件市场；安装时会通过 Claude Code 的 `claude plugin` 命令添加并安装。Claude Code 每次会话启动时都会从那里读取，因此会随 Kero 更新。

如果 `CLAUDE_CONFIG_DIR`、`CODEX_HOME`、`PI_CODING_AGENT_DIR`、`PI_CONFIG_DIR` 或 `XDG_CONFIG_HOME` 改变了智能体文件的位置，Kero 会跟着走，并且不会改动各设置文件里的其他内容。

传入 `--agent` 并指定 `claude`、`codex`、`pi`、`omp` 或 `opencode`，可以只为一个智能体安装。只需安装一次：技能是指向 Kero 自己那份副本的链接，会随 Kero 更新而更新；Pi、OMP 和 OpenCode 的插件，Kero 也会保持为最新。`kero skill status` 会告诉你各部分是否为最新。旧版 Kero 安装的 Claude Code 或 Codex 钩子会显示为“outdated”，因为还没有回合结束的钩子：再运行一次 `kero skill setup` 即可添加。

```sh
kero skill status    # 已安装什么，是否为最新
kero skill show      # 打印技能内容
kero skill remove    # 卸载技能和钩子
```

钩子只对安装之后启动的智能体会话生效，而且智能体可能会先征求你的同意：

- **Claude Code** 只在你信任的文件夹中运行钩子。
- **Codex** 会请你批准新钩子：用 `/hooks` 批准 Kero 的钩子。
- **OpenCode** 在其后台服务启动时加载插件：趁没有会话在忙时，运行一次 `opencode service restart`。

OpenCode 在后台服务而非窗格中运行智能体的命令，所以这些命令拿不到 `KERO_SESSION` 和 `KERO_PANE`。Kero 会改为把窗格告诉智能体，由它显式指定为目标。出于同样的原因，OpenCode 智能体执行的 `kero report` 无法到达它的窗格。

在 Kero 之外，钩子什么也不做。

<a id="how-agents-get-keros-messages"></a>

## 智能体如何收到 Kero 的消息

Kero 会向智能体发送消息：通过[编排器](./orchestrators.md)转达子会话的报告、[PR 关注](./pull-requests.md)、页面上的答案，以及 **发送到会话…** 等。每条消息都会在智能体回到提示符时，通过该智能体自己的接收方式发送一次，不会触碰你正在提示符中输入的内容：

| 智能体      | 消息如何送达                                                               |
| ----------- | -------------------------------------------------------------------------- |
| Claude Code | Kero 的插件会在整个会话期间运行 `kero inbox`，Claude 会将收到的每条消息作为事件处理。长消息会先收到开头，Kero 文件夹中的文件保存着完整内容供 Claude 阅读。 |
| Codex       | `codex queue`，由 Codex 作为独立回合运行。你结束一个回合后，Codex 会保留它，直到你下次输入提示词。 |
| OpenCode    | OpenCode 的后台服务会在回合结束后，将消息作为排队的提示词发送。            |
| Pi、OMP     | Kero 的扩展会将消息作为独立提示词发送，或作为当前回合的后续内容。           |

其他智能体，以及接收方式失败的智能体（例如在安装前启动的智能体），会收到输入到提示符中的消息，并由 Kero 按下 Enter。之后，只要你开始在窗格中输入，Kero 就不会再按 Enter，因此不会发送你自己的文字。

## 智能体能做什么

在身边运行任务：

```sh
kero split --down -- npm run dev
kero new -n review -- claude
```

操控它并读取结果：

```sh
kero prompt review "review the changes in src/auth"
kero wait tests --for idle,error --timeout 10m
kero read tests -n 60
```

在旁边跑测试，并读取失败信息：

```sh
pane=$(kero split --down -- go test ./...)
kero wait "$pane" --exit || kero read "$pane" -n 80
```

查看仓库的工作树，附带 `git worktree list` 没有的信息：每个工作树中的会话、是否由 Kero 创建、删除会丢失什么、自动清理会如何处理；查看自己会话的工作树、设置和拉取请求；把自己的会话连同对话移到新的 git 工作树中，并在本轮结束后在那里继续（[详情](./worktrees.md#moving-a-session-into-a-worktree)）：

```sh
kero worktree list
kero worktree status
kero worktree handoff fix-login --prompt "go on with the fix"
```

处理拉取请求：编写[评审导览](./pull-requests.md#review-guides)，查看会话关联的拉取请求。Kero 会为智能体关注它，在检查失败时向其提示符输入一条以“Kero PR watch:”开头的消息。在你要求时，还能合并、更新或检出拉取请求（[全部命令](./cli.md)）：

```sh
kero pr guide stop src/queue.go:10-42 --title "The new queue" "Why it exists."
kero pr status                  # 关联的拉取请求：检查、冲突
kero pr update-branch --rebase
kero pr merge --squash          # 仅在你要求时：合并无法撤销
kero pr checkout 123 --worktree -- claude
```

告诉你进展如何：

```sh
kero report working "Migrating the database" --progress 40
kero report blocked "Approve the production deploy?"
kero report done "Deployed to staging"
```

`blocked` 报告会把会话变为*等待中*并通知你，所以智能体只应在离开你就无法继续时使用它。

## 运行其他智能体的智能体

- [编排器](./orchestrators.md)是一个智能体会话：它规划工作，为其启动子会话并跟进，而你只需和它交谈。
- [Ama](./ama.md) 是 Kero 的助手代理，替你启动和管理会话、工作树、项目和设置，并回答关于 Kero 的问题。

<a id="remote-control"></a>

## 远程控制

开启**远程控制**后（默认开启，位于**设置 › 智能体**），智能体还能操控应用本身：用 `kero focus` 显示某个会话，用 `kero do` 执行菜单操作，用 `kero browser` 操作浏览器窗格，用 `kero pr review` 开始拉取请求的审查。设置 `remote-control = false` 可将其关闭，让智能体只能操作自己的会话。[Ama](./ama.md#without-remote-control) 不需要它。

## 环境变量

每个窗格都会获得：

| 变量           | 值                             |
| -------------- | ------------------------------ |
| `KERO_SESSION` | 窗格所在会话的 id              |
| `KERO_PANE`    | 窗格的 id                      |
| `KERO_SOCKET`  | kerod 监听的位置，供 `kero` 连接它 |
| `TERM`、`COLORTERM` | `xterm-256color` 和 `truecolor` |
| `TERM_PROGRAM`、`TERM_PROGRAM_VERSION` | `ghostty` 和 Ghostty 的版本：Kero 的终端就是 Ghostty 的终端，因此了解 Ghostty 按键和通知的程序会使用它们 |
| `LC_TERMINAL`、`LC_TERMINAL_VERSION` | `kero`，以及它的协议版本 |
| `SHELL`        | 窗格的 shell；在 Windows 上，仅当它是 POSIX shell（如 bash）时才设置 |
| `LANG`         | `en_US.UTF-8`，当 kerod 没有 `LANG`、`LC_ALL` 和 `LC_CTYPE` 中的任何一个时 |
| `PATH`         | kerod 的 `PATH`，末尾加上 Kero 自己的 `bin` 文件夹，其中有 `kero` |

`env` 设置中的变量在这些之后设置，并优先生效。窗格还会获得 kerod 启动时的其余环境（即应用的环境），但属于其他终端或智能体的变量除外，如 `TMUX`、`VSCODE_*`、`GHOSTTY_*`、`CLAUDECODE` 和 `CLAUDE_CODE_*`。所以请从不含 `NO_COLOR` 的 shell 启动 Kero，否则所有窗格都会失去颜色（[故障排除](./troubleshooting.md#panes-without-colors)）。[Ama](./ama.md) 的窗格还会获得 `KERO_AMA`，工作树的[设置脚本](./worktrees.md#setup-scripts)会获得 `KERO_WORKTREE_PATH`、`KERO_PROJECT_ROOT` 和 `KERO_WORKTREE_BRANCH`。

检查 `$KERO_PANE` 即可判断脚本是否运行在 Kero 中。
