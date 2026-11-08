---
title: 故障排除
description: Kero 把文件和日志放在哪里、如何重启 kerod、更新失败时怎么办，以及如何检查各个部分。
---

# 故障排除

<a id="where-kero-keeps-things"></a>

## Kero 把东西放在哪里

Kero 把你的设置放在一个文件夹中，把它自己的状态放在另一个文件夹中。

**配置文件夹**：`~/.config/kero`（或 `$XDG_CONFIG_HOME/kero`；Windows 上也是如此，如 `C:\Users\<you>\.config\kero`）。`kero config path` 会打印配置文件的路径。

| 文件 | 内容 |
| --- | --- |
| `config` | 你的[设置](./configuration.md) |
| `agents.toml` | [你自己的智能体](./status.md#your-own-agents)（如果有） |
| `ama/` | [Ama 的文件夹](./ama.md#its-folder)：它的指示、你的 `USER.md`、Kero 的文档 |

**数据文件夹**：Windows 上为 `%AppData%\kero`，macOS 上为 `~/Library/Application Support/kero`，Linux 上为 `~/.config/kero`，在 Linux 上它同时也是配置文件夹。`$KERO_HOME` 可以改变它的位置。**设置 › 高级** 会显示它和它的日志。

| 文件 | 内容 |
| --- | --- |
| `kerod.log` | kerod 的[日志](#logs) |
| `state.json` | 项目和会话，kerod 重启后会重新启动它们 |
| `sessions/` | 每个窗格的回滚缓冲 |
| `conversations.json` | 智能体的[过去的对话](./sessions.md#past-conversations) |
| `app.json`、`window-state.json` | 窗口：它的大小和位置、各个边栏、Ama 的窗口 |
| `updater.json` | Kero 上次检查更新的时间，以及你跳过的版本 |
| `reviews/`、`guides/` | [草稿审查](./pull-requests.md#draft-reviews)和[审查导览](./pull-requests.md#review-guides)，每个拉取请求一个文件 |
| `worktree-cleanup.json` | [工作树清理](./worktrees.md#cleaning-up-worktrees)所掌握的工作树信息 |
| `skills/kero/` | Kero 的技能，各智能体的技能链接到这里 |
| `adblock/` | [广告拦截](./panes.md#blocking-ads)的过滤列表 |
| `bin/` | 窗格在其 `PATH` 中找到的 `kero` 命令 |
| `kerod.sock`、`kerod.lock`、`kerod.running` | 应用和 `kero` 如何连接 kerod，以及它是否在运行；`$KERO_SOCKET` 可以改变套接字的位置 |

在 Windows 上，浏览器窗格把 Cookie 和网站数据保存在 `%LOCALAPPDATA%\Kero\WebView2` 中。

Kero 创建的工作树位于各个仓库的 `.kero/worktrees` 中，或 `worktree-dir` 之下（[工作树](./worktrees.md#where-worktrees-go-and-what-they-start-from)）。

<a id="logs"></a>

## 日志

kerod 只写一个日志：数据文件夹中的 `kerod.log`，Windows 上为 `%AppData%\kero\kerod.log`。**设置 › 高级 › 日志** 会在文件管理器中显示它。应用或 `kero` 启动 kerod 时，kerod 在启动（或启动失败）时打印的内容也会写到这里。

kerod 每次运行都以 `--- daemon run ---` 和一行说明其版本和进程的内容开始。它记录会话的启动、关闭和重启后的恢复，工作树的创建和删除，`.worktreeinclude` 的复制和[设置脚本](./worktrees.md#setup-scripts)（`setup script of <worktree> runs in pane <pane>: <command>`，以及设置的结果），`agents.toml` 中的问题（`agents: …`），以及错误。`previous daemon exit was unclean` 表示 kerod 上次结束时没有保存，比如电脑断电时。日志超过 4 MB 时，Kero 会在 kerod 启动时清空它。

`kero daemon serve` 也会把日志写到它的标准错误输出。

<a id="restarting-kerod"></a>

## 重启 kerod

kerod 是运行你的会话的后台服务（[kerod](./sessions.md#kerod)）。更新之后、应用提示它属于另一个版本时，或它不再响应时，请重启它：

- 侧边栏的**重启**按钮，在 Kero 更新后出现：“Kero 已更新：重启后台服务以运行新版本”。
- **设置 › 高级 › 后台服务 › 重启**。
- 命令面板中的**重启后台服务**。
- `kero daemon restart`。

重启会先保存会话，然后在原处重新启动它们，并保留回滚缓冲；智能体会恢复它们的对话。不是智能体的程序会回到 shell。

```sh
kero daemon status       # kerod 是否在运行：进程、运行时长、会话和窗格、套接字
kero daemon restart
kero daemon stop         # 停止它；它下次启动时会话会重新启动
```

`kero` 无法连接 kerod 时以状态码 `4` 退出；应用的侧边栏会显示**无法连接 kerod**。打开应用或运行任何 `kero` 命令都会重新启动 kerod。版本与 kerod 不同的 `kero` 会提示你运行 `kero daemon restart`。

没有会话、也没有任何连接时，kerod 会在 30 秒后自行退出，除非它作为服务运行（`kero daemon serve`）或在监听其他机器；日志中会显示 `idle: exiting`。

## 更新

Kero 打开期间每天检查一次更新，**设置 › 通用 › 检查更新** 会立即检查。**自动下载并安装更新** 会在有更新时安装它，下次打开 Kero 时生效。

**检查失败**（例如 GitHub 返回 `504 Gateway Timeout`）不会改变任何东西：自动检查会静默失败并稍后重试，**检查更新** 会提示无法检查。500 到 504 的状态码是 GitHub 那边的问题，不是你的：请稍后再试。想现在就更新，可以手动安装：

1. 从你的 Kero 获取更新的仓库的发布页面下载安装程序，Windows 上为 `kero-setup-<version>-amd64.exe`：对于这个版本，是 [github.com/FallDownTheSystem/kero/releases](https://github.com/FallDownTheSystem/kero/releases)。
2. 停止 kerod，因为它占用着 `Kero.exe`：**退出并结束所有会话…**，或 `kero daemon stop`。
3. 从文件资源管理器或你自己的终端运行安装程序，不要在 Kero 的窗格或智能体的 shell 中运行（[原因](#panes-without-colors)）。

更新之后，按侧边栏的提示重启 kerod，让你的会话运行新版本。用 `kero -H <host> daemon update` 或 `kero hosts install <host>` 更新某台主机上的 Kero（[远程主机](./remote.md#versions-and-updates)）。

<a id="panes-without-colors"></a>

## 窗格没有颜色

kerod 会把它启动时的环境传给每个窗格。如果它是从设置了 `NO_COLOR` 的 shell 启动的（智能体的 shell 常常如此），每个窗格中的每个程序都会不显示颜色。用**退出并结束所有会话…**或 `kero daemon stop` 退出 Kero，然后从开始菜单、Dock 或你自己的终端重新打开它。shell 为自身设置的其他变量也是同样的道理。

## 智能体

- **智能体的状态不显示、会话没有被命名，或重启后没有接上对话**：没有为它设置 Kero 的钩子。`kero skill status` 会说明已安装什么、是否为最新，`kero skill setup` 会安装它（[安装技能](./agents.md#install-the-skill)）。钩子只对安装之后启动的会话生效；Claude Code 只在你信任的文件夹中运行钩子，Codex 会请你用 `/hooks` 批准它。
- **智能体运行 `kero do`、`kero focus` 或 `kero browser` 失败**：它们需要**远程控制**（**设置 › 智能体**，`remote-control`），并且应用处于打开状态。`kero browser wait-message`、`kero browser devtools` 和 `kero do pane.preview` 不需要。[Ama](./ama.md#without-remote-control) 不需要远程控制。
- **新建会话对话框找不到智能体的命令**：Kero 像你一样在你的 shell 中运行智能体，所以别名只在定义了它的 shell 中有效。`kero config get agent` 会列出各个智能体，`kero config check` 会报告有问题的 `agents.toml`。

## 工作树

- **设置脚本没有运行**：它只对 Kero 创建的工作树运行，而且只运行 `kero.json`（或 `t3.json`）中第一个带 `"runOnWorktreeCreate": true` 的脚本（[设置脚本](./worktrees.md#setup-scripts)）。在工作树的会话中运行 `kero worktree status` 可以查看它的设置；`kerod.log` 会显示运行了什么以及结果如何。不是有效 JSON 的 `kero.json` 会让设置失败。
- **新工作树中缺少某个被忽略的文件**：把它列在主检出的 `.worktreeinclude` 中（[复制被忽略的文件](./worktrees.md#copying-ignored-files)）。
- **工作树删不掉**：`kero worktree list` 会说明删除每个工作树会丢失什么，以及清理对它的判断。清理绝不会删除有改动的工作树、有会话正在使用的工作树，或十分钟内刚创建的工作树。

## 设置

`kero config check` 会报告配置文件和 `agents.toml` 中未知的配置项和无效的值。不带值的 `kero config set <key>` 会把一个配置项恢复为默认值，**设置 › 高级 › 重置所有设置** 则恢复所有配置项，包括键盘快捷键；你的会话会保留。

## 检查各个部分

```sh
kero version                 # kero 及其协议的版本
kero daemon status           # kerod
kero clients                 # 连接到 kerod 的应用和命令
kero config check            # 配置文件和 agents.toml
kero skill status            # 每个智能体的技能和钩子
kero worktree status         # 本会话的工作树、设置和拉取请求
kero hosts check <host>      # 某台主机：连接它，并报告结果
kero browser devtools        # 浏览器窗格的 DevTools 端口
kero ls --json               # 每个会话和窗格，及其状态
```
