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

<a id="orchestrator-sessions"></a>

## 编排器会话

编排器是一个智能体会话：它规划工作，为其启动其他会话（它的**子会话**）并跟进，而你只需和它交谈。在[新建会话对话框](./sessions.md#new-session-dialog)中选择 **编排器** 并可填写 **目标**，或者：

```sh
kero new --orchestrator --goal "上线新的登录页"
kero orchestrator <session>       # 把会话设为编排器；加 off 则撤销
```

所选的智能体（否则为 `default-agent`）以角色提示词开始：规划并委派，用 `kero new` 启动子会话，能答的自己答，只在必要时问你，始终记住目标。可在 **设置 › 智能体 › 编排器**（`orchestrator-prompt`）中替换提示词，其中 `{goal}` 是目标，`{agent}` 是默认智能体。

- **子会话。** 编排器用 `kero new` 启动的会话就是它的子会话，位于同一项目。只有一层：子会话自己的 `kero new` 会为同一编排器再启动一个子会话，而不是孙会话。`kero new --parent <session>` 指定编排器，`--parent none` 启动不属于任何编排器的会话。`orchestrator-max-children` 限制同时运行的子会话数量。
- **报告。** 当子会话开始等待输入、工作后转为空闲、失败、退出或被关闭时，Kero 会在编排器回到提示符时把这些输入给它：接近的变化合并为一条消息，持续不到两秒的状态不报告。子会话可用 `kero report --to-parent "<总结>"` 发送自己的结果，随下一次报告一起送达。编排器自己关闭的子会话不会被报告。
- **侧边栏。** 编排器的行带有标记，提示中显示其目标；子会话无论如何移动都紧跟在它下方，每个前面有 ↳。用它的展开箭头可折叠子会话（Kero 会记住）；折叠时会显示需要你处理的子会话数量。
- **通知。** 子会话的变化会更新侧边栏，但不通知你，也不计入 Dock 徽标：编排器会得知它们。编排器自己的等待和错误照常通知。`notify-children`（**设置 › 通知**）可恢复子会话的通知。
- **你仍可直接与子会话交谈**：点击它并输入。编排器会从下一次报告得知。
- **关闭**编排器时会询问是同时关闭子会话，还是保留为顶层会话（`kero close <orchestrator> --children` 会一并关闭）；拥有专属工作树的子会话会保留工作树，交给[清理](./changes.md)。关闭子会话会把它移出树，工作树中的子会话照常询问如何处理工作树。

`kero ls` 会标出编排器和子会话；`kero status <orchestrator>` 显示其目标和子会话，`kero status <child>` 显示其编排器。

## 远程控制

开启**远程控制**后（默认开启，位于**设置 › 智能体**），智能体还能操控应用本身：用 `kero focus` 显示某个会话，或用 `kero do` 执行菜单操作。设置 `remote-control = false` 可将其关闭，让智能体只能操作自己的会话。

## 环境变量

每个窗格都会获得：

| 变量           | 值                             |
| -------------- | ------------------------------ |
| `KERO_SESSION` | 窗格所在会话的 id              |
| `KERO_PANE`    | 窗格的 id                      |

检查 `$KERO_PANE` 即可判断脚本是否运行在 Kero 中。
