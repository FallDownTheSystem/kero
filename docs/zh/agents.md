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
- 一个**钩子**，在智能体开始会话时运行。它告诉智能体自己位于哪个 Kero 会话和窗格，省得它花时间去查；如果会话还没有名字，就让智能体按任务给它命名，例如 `fix-login-redirect`，不用你动手。它还会把智能体自己的会话告诉 Kero，这样 kerod 重启后（比如电脑重启之后），智能体就能[接上之前的对话](./sessions.md#kerod)。

对于你已安装的智能体，钩子会放在各自查找钩子的位置：

| 智能体      | 钩子位置                                              |
| ----------- | ----------------------------------------------------- |
| Claude Code | `~/.claude/settings.json` 中的 `SessionStart` 钩子     |
| Codex       | `~/.codex/hooks.json` 中的 `SessionStart` 钩子         |
| Pi          | 扩展 `~/.pi/agent/extensions/kero.ts`                  |
| OMP         | 扩展 `~/.omp/agent/extensions/kero.ts`                 |
| OpenCode    | 插件 `~/.config/opencode/plugins/kero`                 |

如果 `CLAUDE_CONFIG_DIR`、`CODEX_HOME`、`PI_CODING_AGENT_DIR`、`PI_CONFIG_DIR` 或 `XDG_CONFIG_HOME` 改变了智能体文件的位置，Kero 会跟着走，并且不会改动各设置文件里的其他内容。

传入 `--agent` 并指定 `claude`、`codex`、`pi`、`omp` 或 `opencode`，可以只为一个智能体安装。只需安装一次：技能是指向 Kero 自己那份副本的链接，会随 Kero 更新而更新；Pi、OMP 和 OpenCode 的插件，Kero 也会保持为最新。`kero skill status` 会告诉你各部分是否为最新。

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

查看仓库的工作树，附带 `git worktree list` 没有的信息：每个工作树中的会话、是否由 Kero 创建、删除会丢失什么、自动清理会如何处理；查看自己会话的工作树、设置和拉取请求；把自己的会话连同对话移到新的 git 工作树中，并在本轮结束后在那里继续（[详情](./changes.md#moving-a-session-into-a-worktree)）：

```sh
kero worktree list
kero worktree status
kero worktree handoff fix-login --prompt "go on with the fix"
```

处理拉取请求：编写[评审导览](./changes.md#review-guides)，查看会话关联的拉取请求。Kero 会为智能体关注它，在检查失败时向其提示符输入一条以“Kero PR watch:”开头的消息。在你要求时，还能合并、更新或检出拉取请求（[全部命令](./cli.md)）：

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

## 助手代理 Ama

Ama 是替你操作 Kero 的智能体，名字来自日本树蛙（雨蛙，*amagaeru*）。点击会话边栏底部的青蛙，或按 **Ctrl+Shift+A**（macOS 上为 **⇧⌘A**），或选择**显示 › Ama**，然后这样请它：

- “为项目 X 中每个打开的 PR 启动代码评审。”
- “清理所有旧的工作树。”
- “为这个文件夹添加新项目，并用这个目标在那里启动一个编排器。”
- “哪些会话需要我？”
- “关闭项目 Y 中所有已完成的会话。”

Ama 是在真实终端中运行的普通智能体会话。它运行你的默认智能体，或你在**设置 › 智能体 › Ama** 中选择的智能体（`ama-agent`）。它自己不做项目工作：真正的工作，它会在合适的项目中启动会话或编排器。它不属于任何项目，在 Kero 配置文件旁的专属文件夹 `ama`（`~/.config/kero/ama`）中运行。边栏不列出它，“需要你”不计入它，它也不发通知。Ama 等待你时，青蛙上会显示一个圆点。

关闭弹出框后 Ama 继续运行，下次打开时仍是同一对话。在弹出框中：

- **新对话**开始一段新对话。
- **历史**回到 Ama 之前的某段对话。
- **清屏**清除屏幕。

Kero 把 Ama 的指示写入它的文件夹，作为 `CLAUDE.md` 和 `AGENTS.md`，并在 Kero 更新时重写。你自己的指示写在旁边的 `USER.md` 中：Kero 只创建一次该文件，之后从不修改，Ama 会先读它。可从**设置 › 智能体 › Ama** 打开。Ama 启动前，Kero 还会像 `kero skill setup` 那样为 Ama 的智能体安装技能。

Ama 用[管理 Kero](./cli.md#managing-kero) 中的命令管理 Kero。

## 远程控制

开启**远程控制**后（默认开启，位于**设置 › 智能体**），智能体还能操控应用本身：用 `kero focus` 显示某个会话，或用 `kero do` 执行菜单操作。设置 `remote-control = false` 可将其关闭，让智能体只能操作自己的会话。

Ama 不需要远程控制：kerod 只给 Ama 的窗格一个秘密值 `$KERO_AMA`，即用 kerod 每次启动时重新生成的密钥对 Ama 的会话做的哈希。`kero do`、`kero focus` 和 `kero browser` 会传递它，只有 kerod 确认它属于 Ama 时，应用才会在没有远程控制的情况下执行操作。它不存入任何文件，其他窗格拿不到，对其他机器的 kerod 也无意义，所以其他智能体仍由远程控制设置决定。

## 环境变量

每个窗格都会获得：

| 变量           | 值                             |
| -------------- | ------------------------------ |
| `KERO_SESSION` | 窗格所在会话的 id              |
| `KERO_PANE`    | 窗格的 id                      |

检查 `$KERO_PANE` 即可判断脚本是否运行在 Kero 中。
