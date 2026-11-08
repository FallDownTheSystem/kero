---
title: 工作树
description: 为每个智能体提供专属的 git 工作树，用脚本设置新工作树，并清理它们。
---

# 工作树

多个智能体在同一个检出目录里并行工作，会被彼此的改动绊住。在专属 git 工作树中运行的会话，在自己的分支、自己的文件夹中工作，因此它的智能体可以编辑、构建和提交，而不会碰到你的文件。

## 在工作树中启动会话

**会话 › 在工作树中新建会话…**（<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd>）会打开已开启**在工作树中**的[新建会话对话框](./sessions.md#new-session-dialog)，在一个检出你所选分支，或检出你命名并从仓库默认分支创建的新分支的工作树中启动会话。侧边栏中项目的菜单里也有这一项，用于为该项目的仓库启动会话。在没有菜单栏的 Windows 和 Linux 上，菜单栏的菜单位于侧边栏设置旁边的**菜单**按钮（⋯）中。

会话会加入仓库所在的项目，边栏中会显示它的分支。其他启动方式：

- 历史标签页[工作树分区](#the-worktrees-section)中的 **+**，或提交菜单中的**从这里在工作树中新建会话…**和**在新工作树中打开 …**。
- 分支切换器中分支的菜单，可以在专属工作树的会话中打开该分支。
- 拉取请求的**审查**和**在新工作树中检出**（[拉取请求](./pull-requests.md)）。
- 智能体把自己的会话移入工作树（[见下文](#moving-a-session-into-a-worktree)）。
- 命令行：

```sh
kero new --worktree fix-login -- claude   # 在专属工作树中、以新分支启动会话
kero new --worktree fix-login --base dev  # 从 dev 创建分支
kero worktree create fix-login -p api -- claude "fix the login"   # 在某个项目的仓库中
```

<a id="where-worktrees-go-and-what-they-start-from"></a>

## 工作树的位置与起点

- 工作树位于仓库内的 `.kero/worktrees/<branch>`，Kero 通过 `.git/info/exclude`（而不是你的 `.gitignore`）忽略它。设置 `worktree-dir` 可以把工作树放到别处。
- 创建工作树前，Kero 会先获取基础分支并从远程的分支创建，所以本地 `main` 过时也没关系。离线时，或本地分支有远程没有的提交时，从本地分支创建。设置 `worktree-from-origin = false` 可以始终从本地分支创建。
- 设置 `worktree-base = current`，可以从项目文件夹所在的分支而不是默认分支创建新分支。

这些设置都在 **设置 › Git** 中。在会话中运行 `kero worktree status`，会打印新工作树采用的是哪些设置。

Kero 创建工作树时，按以下顺序设置它：

1. 获取基础分支，并检出工作树。
2. 复制 [`.worktreeinclude`](#copying-ignored-files) 列出的被忽略文件。
3. 如果工作树有 git 子模块，检出它们。
4. 如果仓库有[设置脚本](#setup-scripts)，运行它：与会话的智能体同时运行，或在智能体之前运行。

窗口右下角的卡片会显示每一步的进度，卡片上的**取消**可以停止设置。

<a id="copying-ignored-files"></a>

## 复制被忽略的文件：`.worktreeinclude`

新工作树只有 git 跟踪的文件。要带上被忽略的文件，如 `.env`、`.claude/settings.local.json` 或 `CLAUDE.local.md`，就像 Claude Code 的工作树那样，在仓库主检出的顶层 `.worktreeinclude` 里按 `.gitignore` 的语法列出它们：

```gitignore
.env
.env.local
.claude/settings.local.json
CLAUDE.local.md
config/secrets/
```

Kero 读取主检出中的 `.worktreeinclude`（无论是否已提交），并从主检出复制每个匹配且被 git 忽略的文件；已跟踪的文件本来就在工作树中。工作树里已有的文件保持不变。它跳过 `node_modules` 和其他工作树，只有当某个模式指向整个被忽略的文件夹内的路径（如 `cache/keep.txt`）时才进入该文件夹。[清理](#cleaning-up-worktrees)不会把这些副本算作工作树自己的内容，因为它们属于主检出。

<a id="setup-scripts"></a>

## 设置脚本：`kero.json`

要为新工作树做准备（比如安装依赖或构建），在仓库顶层放一个 `kero.json`，写上在创建工作树时运行的脚本：

```json
{
  "scripts": [
    { "name": "Install", "command": "bun install", "runOnWorktreeCreate": true }
  ]
}
```

| 键 | 含义 |
| --- | --- |
| `name` | 设置卡片上显示的脚本名称（默认：文件名） |
| `command` | 命令，写法与你在 shell 中输入的一样：`npm ci && npm run build` |
| `runOnWorktreeCreate` | 设为 `true` 则在创建工作树时运行。Kero 只运行第一个带有它的脚本 |
| `async` | 设为 `false` 时，智能体会等脚本结束后再启动。省略或设为 `true` 时，智能体立即在旁边启动 |

脚本在会话自身终端下方的终端中、在工作树的文件夹里、通过你的 shell（`shell` 设置）运行，并设置以下变量：

| 变量 | 值 |
| --- | --- |
| `KERO_WORKTREE_PATH` | 新工作树的文件夹 |
| `KERO_PROJECT_ROOT` | 仓库的主检出，可从中复制或链接内容 |
| `KERO_WORKTREE_BRANCH` | 工作树的分支 |

- **设为 `"async": false` 时**，脚本的终端先启动，脚本结束后智能体在它上方启动；即使脚本失败也会启动，以便智能体处理。当智能体需要设置先完成时使用，比如运行测试所需的依赖。
- **否则**，智能体立即启动，脚本在它旁边运行。
- 脚本成功或你取消它时，终端会关闭；失败时会连同输出一起保留。
- 如果你的 shell 是 `cmd`，Kero 无法判断命令何时结束：脚本会继续运行，等待它的智能体会立即启动。

Kero 先在新工作树中查找脚本，再在主检出中查找；在每处都先找 `kero.json`，再找 `t3.json`：Kero 也读取格式相同的 [t3code](https://github.com/pingdotgg/t3code) 的 `t3.json`，并像 t3code 那样为其脚本提供 `T3CODE_PROJECT_ROOT` 和 `T3CODE_WORKTREE_PATH`。因此，提交 `kero.json` 可以让所有人共享设置；也可以只在你自己的主检出中放一个并忽略它，供自己使用。不是有效 JSON 的文件会让设置失败，卡片上会说明原因。

Kero 为会话创建的每个工作树都会运行它：来自新建会话对话框、`kero new --worktree`、`kero worktree create`、`kero worktree handoff`，以及在新工作树中审查或检出拉取请求。你自己用 `git worktree add` 创建的工作树不会运行。

要查看设置的情况，在会话中运行 `kero worktree status`，它会打印设置的各个步骤；或者查看 `kerod.log`（[故障排除](./troubleshooting.md#logs)），其中记录了 `setup script of <worktree> runs in pane <pane>: <command>` 以及每次设置的结果。

## 关闭工作树中的会话

关闭会话时，Kero 会询问是否一并删除它的工作树：

- **没有未提交的改动：** 默认是“删除工作树”。分支若已合并会一并删除，否则会连同提交一起保留。
- **有未提交的改动：** Kero 会说明有多少文件会丢失，默认是“保留工作树”。
- **还有其他会话在这个工作树中工作：** 工作树会保留，Kero 不会询问。

```sh
kero close fix-login --remove-worktree           # 关闭会话并删除工作树
kero close fix-login --remove-worktree --force   # 即使有未提交的改动
```

<a id="moving-a-session-into-a-worktree"></a>

## 把会话移入工作树

已经在你的检出中工作的代理，可以把自己的会话连同对话一起移入专属的工作树，而不必新建会话。代理在自己的窗格中运行 `kero worktree handoff <branch>`，Kero 就会像为新会话那样创建工作树（新分支默认从默认分支创建，或按 `worktree-base` 的设置，或从 `--base`），并把它归给该会话。代理的这一轮结束后，Kero 会在工作树的同一文件夹中重新启动代理、恢复它的对话，并输入给定的 `--prompt`（如果有）。它的其他窗格和未提交的改动都留在原处。Kero 的技能会教代理使用它。已经有专属工作树的会话不能再次移动。

```sh
kero worktree handoff fix-login                          # 从默认分支创建
kero worktree handoff fix-login --base HEAD --prompt "go on with the fix"
```

<a id="the-worktrees-section"></a>

## 工作树分区

历史标签页中储藏下方的**工作树**分区列出仓库的工作树，并标出删除每个工作树会丢失什么：改动的文件和不在任何远程上的提交以橙色显示；什么都不会丢失时，以绿色显示*已合并*或*已推送*。**推送**推送工作树的分支，**在会话中打开**显示在其中工作的会话或新开一个，**删除工作树**会先询问：有改动的工作树只有在你选择**仍然删除**时才会删除，有未推送提交的会保留其分支，没有任何损失的可以**连同分支删除**。为某个会话创建的工作树会随该会话一起关闭。它的菜单还可以复制路径、把它的分支合并到当前分支，**清理丢失的工作树**会忘掉文件夹已被删除的工作树。**+** 在新工作树中开始一个会话。

分支切换器的**工作树**组中也会列出工作树：双击某个工作树即可打开它的会话。

<a id="cleaning-up-worktrees"></a>

## 清理工作树

工作树会越积越多。打开 **设置 › Git › 工作树清理**，Kero 会在启动时、每小时一次以及你点击 **立即删除** 时，删除它创建的工作树。默认关闭。符合你所选任一规则的工作树会被删除：

- **闲置时间**：这么多天里没有提交、检出或会话。
- **拉取请求已合并**：分支在 GitHub 上的拉取请求已合并，且合并的是工作树所在的提交。Kero 通过 `gh` 查询，需要已登录 `gh`。
- **没有自己的提交**：分支上没有默认分支之外的提交。
- **最后一个会话已关闭**：已没有会话在其中工作，比如你关闭会话时保留了工作树。

Kero 只删除它自己文件夹（`.kero/worktrees` 或 `worktree-dir`）中的工作树，绝不删除有会话正在使用的、被 git 锁定的、十分钟内刚创建的，或含有自己内容（未提交的改动、未跟踪的文件、构建无法重新生成的被忽略文件）的工作树。与主检出内容相同的被忽略文件（如 `.worktreeinclude` 复制来的），以及 `node_modules`、`dist`、`build`、`target`、`.venv` 等文件夹不算在内。默认分支已包含其全部提交的分支会随工作树一起删除，否则连同提交一起保留。

**项目** 可为这台电脑上的某个项目设置自己的规则（**自定义**），或不清理（**关闭**，保留其工作树）。只要仓库有一个项目设为关闭，无论其他项目如何设置，都会保留其工作树。

## 在命令行中

```sh
kero worktree list -p api        # 工作树：其中的会话、是否由 Kero 创建、删除会丢失什么、清理的判断
kero worktree status             # 本会话的工作树、它的设置、拉取请求，以及能否移交
kero worktree create fix-login -p api -- claude "fix the login"
kero worktree remove fix-login -p api --branch   # 像历史标签页那样删除，连同分支
kero worktree cleanup --dry-run  # 自动清理现在会删除什么
kero worktree cleanup
kero worktree prune -p api       # 忘掉文件夹已不存在的工作树
```

`-d <folder>` 用仓库中的某个文件夹指定仓库，代替 `-p <project>`；两者都不给时，为当前文件夹所在的仓库。`kero worktree remove` 像历史标签页那样删除工作树：Kero 为某个会话创建的工作树会连同该会话一起关闭，其他会话正在使用的则保留。`--json` 以 JSON 输出 `list`、`status` 和 `cleanup` 的结果。
