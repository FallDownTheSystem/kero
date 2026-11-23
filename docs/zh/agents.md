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

- 一个**技能**，教智能体使用命令行。Claude Code 从 `~/.claude/skills/kero` 读取，CodeBuddy Code 从 `~/.codebuddy/skills/kero` 读取；Codex、OpenCode 等智能体从 `~/.agents/skills/kero` 读取。它的 `SKILL.md` 很短：列出旁边 `reference/` 中的参考文件，并说明各在何时阅读，例如启动子会话前读 `orchestrating.md`，向编排器报告前读 `reporting.md`，交接或合并前读 `worktrees-prs.md`。智能体只读任务需要的那些。
- 一个**钩子**，在智能体开始会话时运行。它告诉智能体自己位于哪个 Kero 会话和窗格，省得它花时间去查；如果会话还没有名字，就让智能体按任务给它命名，例如 `fix-login-redirect`，不用你动手。它还会把智能体自己的会话告诉 Kero，这样 kerod 重启后（比如电脑重启之后），智能体就能[接上之前的对话](./sessions.md#kerod)。智能体每结束一个回合，同一个钩子也会运行，把回合的结束和最后的回复告诉 Kero，这样[编排器](./orchestrators.md)会在子会话的回合结束时得知，而不是等它的屏幕看起来空闲时。此时钩子不输出任何内容，并在后台运行，不会拖慢智能体。

对于你已安装的智能体，钩子会放在各自查找钩子的位置：

| 智能体         | 钩子位置                                                                                         |
| -------------- | ------------------------------------------------------------------------------------------------ |
| Claude Code    | `~/.claude/settings.json` 中的 `SessionStart`、`Stop`、`StopFailure`、`UserPromptSubmit` 钩子，`Bash` 的 `PreToolUse`、`PostToolUse`、`PostToolUseFailure` 钩子，以及 Kero 的插件 `kero@kero` |
| CodeBuddy Code | `~/.codebuddy/settings.json` 中的 `SessionStart` 钩子                                            |
| Codex          | `~/.codex/hooks.json` 中的 `SessionStart`、`Stop`、`Interrupt` 钩子，以及 `Bash` 的 `PreToolUse`、`PostToolUse` 钩子 |
| Pi             | 扩展 `~/.pi/agent/extensions/kero.ts`                                                            |
| OMP            | 扩展 `~/.omp/agent/extensions/kero.ts`                                                           |
| PiG            | 扩展 `~/.pig/agent/extensions/kero.ts`                                                           |
| OpenCode       | 插件 `~/.config/opencode/plugins/kero`                                                           |

Claude Code 的插件位于 Kero 自己的文件夹中，是名为 `kero` 的插件市场；安装时会通过 Claude Code 的 `claude plugin` 命令添加并安装。Claude Code 每次会话启动时都会从那里读取，因此会随 Kero 更新。

如果 `CLAUDE_CONFIG_DIR`、`CODEBUDDY_CONFIG_DIR`、`CODEX_HOME`、`PI_CODING_AGENT_DIR`、`PI_CONFIG_DIR`、`PIG_CODING_AGENT_DIR`、`PIG_HOME` 或 `XDG_CONFIG_HOME` 改变了智能体文件的位置，Kero 会跟着走，并且不会改动各设置文件里的其他内容。

传入 `--agent` 并指定 `claude`、`codebuddy`、`codex`、`pi`、`omp`、`pig` 或 `opencode`，可以只为一个智能体安装。只需安装一次：技能的每个文件都是指向 Kero 自己那份副本的链接（在未开启开发者模式的 Windows 上是副本），会随 Kero 更新而更新，新版技能增加的文件会被加上，删去的文件会被移除，而你放在技能文件夹里的自己的文件保持不动；Pi、OMP、PiG 和 OpenCode 的插件，Kero 也会保持为最新。`kero skill status` 会告诉你各部分是否为最新。旧版 Kero 安装的 Claude Code 或 Codex 钩子会显示为“outdated”，因为还没有回合结束或[构建](#builds-and-tests)的钩子：再运行一次 `kero skill setup`，或在 **设置 › 智能体** 中点 **更新** 即可添加。

```sh
kero skill status    # 已安装什么，是否为最新
kero skill show      # 打印 SKILL.md；kero skill show reporting 打印 reference/reporting.md
kero skill remove    # 卸载技能和钩子，保留你自己的文件
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

你自己发送的内容（页面的回答、**发送到会话…** 和评审的 **发送给智能体**）不会以事件的形式送达 Claude Code：Claude 不会把事件当作你说的话，也就不会按你的回答行事。这些内容会改为输入到 Claude 的提示词中，作为你的输入。

只要 Claude Code 的回合结束，它就回到提示符，即使它放到后台的子智能体或 shell 命令仍在运行：这时你可以输入，Kero 也可以。由于智能体仍在工作，并会在自己的下一个回合中处理这些结果，它在侧边栏和 `kero wait` 中的状态仍是 **working**。Kero 通过 Claude Code 的 `Stop` 钩子得知回合何时结束，并通过 `UserPromptSubmit` 钩子或发送提示词的 Enter 得知下一个回合何时开始。如果钩子来自还没有 `Stop` 钩子的旧版 Kero，在你重新运行 `kero skill setup` 之前，消息会等到后台工作完成。

其他智能体，以及接收方式失败的智能体（例如在安装前启动的智能体），会收到输入到提示符中的消息，并由 Kero 按下 Enter。之后，只要你开始在窗格中输入，Kero 就不会再按 Enter，因此不会发送你自己的文字。

智能体也可以用 `kero message <session> "<text>"`（`-` 表示从标准输入读取）以同样的方式给其他会话的智能体发消息：接收方会看到发送方，例如“your orchestrator, session …”，因此不会像 `kero prompt` 那样把它当成你的话。`kero prompt` 和 `kero key` 仍用于回答智能体在提示符中提出的问题或权限请求，Kero 的消息会等这些处理完。

想知道某条消息为什么被输入到提示符：`kero status --json` 会在 Kero 的监听器接收消息期间，在智能体的窗格上显示 `"inbox": true`；`kerod.log` 会为每条消息记录：何时等待及原因（智能体正在工作，或在等待权限）、如何送达智能体；被输入的消息还会记录原因：没有监听器、你自己的话，或智能体的接收方式失败；无法送达的消息会记录原因，例如窗格已关闭。窗格关闭也会记录，并写明是谁关闭的：你在应用中、`kero close` 及其运行所在的窗格，或窗格的程序退出。kerod 刚启动时，发给 Claude Code、Pi 或 OMP 的消息最多会等 15 秒，让监听器重新连上，然后才输入。在 Windows 上，用 npm 安装的 Codex 无法通过 `codex.cmd` 传递消息，因此改用其软件包自带的 `codex.exe`；Codex 的接收方式失败后，会在五分钟后再次尝试。

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

`blocked` 报告会把会话变为*等待中*并通知你，所以智能体只应在离开你就无法继续时使用它。技能要求智能体让这一行保持最新，不超过七个词：正在做的事、达成的结果（写“Fixed login redirect loop”，而不是“Done”），或正在等待的问题。

<a id="builds-and-tests"></a>

## 构建与测试

多个智能体同时构建或运行测试，无论会话有多少，都会拖慢整台机器。因此 Kero 让所有会话加起来同时只运行 `heavy-slots` 个构建和测试（默认 2 个，**设置 › 智能体 › 构建与测试**）：

- **Claude Code 和 Codex** 无需额外操作：Kero 的 `PreToolUse` 钩子会让构建或测试的 shell 命令（如 `go test`、`npm run build`、`cargo check`、`pytest`、`make`）在开始前等到有空闲槽位，`PostToolUse` 钩子在结束时归还槽位。等待发生在命令之前，所以既不计入智能体的命令超时，也不计入测试自身的超时。等待期间，侧边栏中会话的行会显示*等待构建槽位*及排队位置，智能体也会被告知等了多久。
- **其他智能体和脚本**包装命令：`kero lock heavy -- go test ./...`。它在命令运行期间占用一个槽位，无论命令如何结束都会归还；其中嵌套的 `kero lock heavy` 会立即运行。

不会无限等待，也不会因槽位而失败：命令最多等待 9 分钟（或 `kero lock heavy --timeout`），之后照常运行。命令结束、智能体的回合结束、窗格关闭时槽位都会归还，被放到后台的构建则在 30 分钟后归还。开发服务器和监视器（`npm run dev`、`--watch`、`go run`）不计入。用 `heavy-command` 添加你自己的命令，或加 `!` 排除。`heavy-slots = 0` 关闭此功能。

在 Windows 上，Kero 还会以低于正常的优先级运行智能体及其启动的一切（`lower-agent-priority`），让桌面和 Kero 在构建时保持流畅。WSL 或 Docker 中的工作不受影响。

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
