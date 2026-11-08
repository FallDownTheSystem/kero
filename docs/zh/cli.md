---
title: 命令行
description: 用 kero 命令行，从 shell 或脚本中操控 Kero 的会话和窗格。
---

# 命令行

`kero` 命令可以在任何 shell 中操控会话：你自己的、脚本的，或者智能体的。通过 **Kero › 安装命令行工具…** 安装它。

## 目标

大多数命令都接受一个**目标**：一个会话或一个窗格。目标可以是：

- 一个 id，或它任意一个唯一的前缀
- 会话的名称
- 一个会话，此时指它当前聚焦的窗格

在 Kero 中，`$KERO_SESSION` 和 `$KERO_PANE` 指向命令所在的会话和窗格，也是默认的目标。

## 会话和窗格

```sh
kero ls                          # 列出所有会话及其窗格
kero new -n api -d ~/code/api    # 新建会话，并打印它的 id
kero new -- npm run dev          # 新建一个运行某条命令的会话
kero split --down -- npm test    # 在当前窗格旁新建一个窗格
kero split --url localhost:3000  # 新建一个显示网页的浏览器窗格
kero split --file src/main.go    # 新建一个文件的编辑器窗格
kero split --file plan.md --preview   # 按渲染后的样子显示 Markdown 文件
kero split --url docs/brief.html # 本地 HTML 页面
kero new --file notes/plan.md    # 新建独立的编辑器会话；保存时创建文件
kero rename api backend          # 重命名会话
kero focus backend               # 在 app 中显示它
kero close backend               # 结束会话（如果给的是窗格，则关闭该窗格）
kero close fix-login --remove-worktree   # 同时删除它专属的工作树
kero restart <pane>              # 重新启动已退出窗格的 shell
kero worktree list               # 工作树：其中的会话、是否由 Kero 创建、删除会丢失什么、清理
kero worktree status             # 本会话的工作树、设置、拉取请求，以及能否移交
kero worktree handoff fix-login  # 把这个代理的会话移入专属工作树
kero new --orchestrator --goal "上线登录页"   # 启动编排器，由它启动子会话
kero orchestrator backend [on|off]  # 把会话设为编排器，或改回普通会话
kero close boss --children       # 关闭编排器及其子会话
kero report --to-parent "完成：测试通过"   # 子会话把总结发给编排器
```

`kero worktree handoff <branch>` 接受 `--base <ref>`（新分支的起点）和 `--prompt <text|->`（代理在工作树中重新启动后接着做的事）（[详情](./changes.md#moving-a-session-into-a-worktree)）。

`kero new` 的选项：

| 参数                        | 含义                                            |
| --------------------------- | ----------------------------------------------- |
| `-p`, `--project <name\|id>` | 放入哪个项目（默认按目录决定）                  |
| `-n`, `--name <name>`       | 为会话命名                                      |
| `-d`, `--dir <dir>`         | 在该目录启动（默认为当前目录）                  |
| `-f`, `--focus`             | 在 app 中显示它                                 |
| `-w`, `--wait`              | 等待它的命令结束，并以该命令的退出码退出        |
| `-u`, `--url <url>`         | 打开显示该网页的浏览器，而不是 shell；本地文件的路径则显示该文件 |
| `--file <path>`             | 打开该文件的编辑器，而不是 shell                |
| `--preview`                 | 对 `--file` 的 Markdown 文件，按渲染后的样子显示，而不是文本 |
| `--worktree <branch>`       | 在专属的 git 工作树中、以新分支启动（[详情](./changes.md#sessions-in-their-own-worktree)） |
| `--base <ref>`              | 工作树分支的起点（默认：默认分支）              |
| `--orchestrator`            | 启动[编排器](./agents.md#orchestrator-sessions)：其智能体（给定的命令或 `default-agent`）以角色提示词开始 |
| `--goal <text>`             | 编排器的目标                                    |
| `--parent <session\|none>`  | 会话所属的编排器（默认：运行命令的会话所属的编排器）；`none` 表示不属于任何编排器 |

## 输入与读取

```sh
kero send api "npm run migrate" --enter   # 输入文本，然后按回车
kero send api -e 'y\n'                    # 解析 \n \r \t \e \xHH 转义
kero key api ctrl+c                       # 按键
kero read api -n 40                       # 屏幕上最后 40 行
kero read api -a                          # 屏幕内容连同回滚缓冲
```

`kero key` 支持 `enter`、`tab`、`escape`、`up`、`down`、`left`、`right`、`backspace`，以及 `ctrl+c`、`ctrl+d` 这类带修饰键的组合。

## 等待

```sh
kero wait api                             # 等到它不再工作
kero wait api --for waiting,error --timeout 10m
kero prompt review "summarize the diff"   # 发送并回车，等待，然后打印屏幕
```

`kero wait` 返回时会打印窗格的状态。`kero prompt` 发送文本并回车，等到窗格不再工作，再打印它的屏幕。这是向另一个智能体提问最快的方式。

### 等待命令结束

像 `make test` 这样的命令不会显示为*工作中*，所以要等它跑完，就用 `--wait` 启动它。`kero` 随后会以该命令自身的退出码退出：

```sh
if kero split --down --wait -- make test; then
  echo "tests passed"
fi
```

对于一个已经在运行启动时所带命令的窗格，`kero wait --exit` 起同样的作用：

```sh
pane=$(kero split --down -- go test ./...)
# … 做些别的事 …
kero wait "$pane" --exit --timeout 10m
echo "go test exited with $?"
```

命令结束后，窗格连同输出会保留下来，你可以用 `kero read` 读取。如果 shell 是 `cmd`，则无法等待命令结束，因为它判断不出命令何时结束。

## 监听事件

`kero events` 会在会话和窗格发生变化时立即打印出来，每个事件一行，直到你停止它。脚本可以借此同时跟踪所有智能体，而不必轮询 `kero status`。

```sh
kero events                               # 所有会话
kero events api                           # 一个会话，或一个窗格
kero events -p kero                       # 一个项目的会话
kero events --for waiting,error           # 只看进入这些状态的窗格
kero events --json | jq -r .name          # 每行一个 JSON 对象
kero events --for waiting --once          # 第一个事件之后就退出
```

```
12:04:31  api x78n9ss  working → waiting  Allow Bash: rm -rf dist?
12:04:40  api  renamed api → backend
```

| 事件 | 何时 | 字段 |
| --- | --- | --- |
| `pane.status` | 窗格的状态改变 | `pane`、`from`、`to`、`message`、`kind`、`agent`；变为 exited 时还有 `exitCode` |
| `command.exited` | 窗格启动时所带的命令结束，shell 继续运行 | `pane`、`exitCode` |
| `pane.created`、`pane.closed` | 窗格启动或关闭 | `pane` |
| `session.created`、`session.closed` | 会话启动或关闭 | |
| `session.renamed` | 会话改名 | `from`、`to` |
| `session.moved` | 会话移到另一个项目 | `from`、`to`（项目的 ID） |
| `notice` | 会话中的程序发出桌面通知 | `pane`、`title`、`body` |

加上 `--json` 时，每个事件还带有 `event`、`time`（unix 毫秒）、`session`、`project` 和 `name`（会话在侧边栏中显示的名字）。

`kero events` 只报告从现在开始的变化；已有的内容请用 `kero ls --json` 查看。指定目标时，目标关闭后它就会停止。`--once` 在第一个匹配的事件之后退出；如果目标先关闭或超过 `--timeout`，则以 `1` 退出。kerod 在状态稳定后才发送它，所以只持续一瞬间的状态（比如短暂的工作中）可能不会出现。

<a id="browsing"></a>
## 浏览网页

`kero browser` 操控浏览器窗格中的网页，就像 `kero send` 和 `kero read` 操控终端一样：点击、输入、读取页面、截图。它的目标是一个浏览器窗格，或者一个会话（指其中的浏览器）。不指定目标时，它操控所在会话的浏览器。

```sh
kero split --url localhost:3000            # 打开网页，并打印窗格的 id
kero browser open localhost:3000/todos     # 前往某个网页，并等待加载完成
kero browser type '#new-todo' "Buy milk" --enter
kero browser wait '.todo-list li'          # 等到某个元素出现
kero browser wait --text "1 item left"     # 等到某段文本出现
kero browser text                          # 页面的文本
kero browser text h1                       # 某个元素的文本
kero browser click 'button[type=submit]'
kero browser screenshot -o page.png
kero browser eval 'document.title'
kero browser console --errors              # 页面抛出的错误和日志，附文件和行号
kero browser network --failed              # 失败或返回 4xx/5xx 的请求
kero browser emulate device iphone-15      # 像手机那样显示页面
```

| 命令                            | 作用 |
| ------------------------------- | ---- |
| `open <url>`                    | 前往某个网页，并等待加载完成 |
| `back`, `forward`, `reload`     | 后退、前进或重新加载页面，并等待完成 |
| `text [selector]`               | 打印页面的可见文本，或第一个匹配元素的文本 |
| `html [selector]`               | 打印页面的 HTML，或该元素的 HTML |
| `click <selector>`              | 像鼠标一样点击该元素 |
| `type <selector> <text…>`       | 向输入框输入文本，替换原有内容；`--enter` 会在之后按回车，从而提交所在表单 |
| `wait [selector] [--text text]` | 等到元素或文本出现；两者都不给时，等到页面加载完成 |
| `eval <js…>`                    | 在页面中运行 JavaScript 并打印结果：字符串原样输出，其他值输出为 JSON |
| `screenshot [-o file.png]`      | 保存页面截图，并打印保存位置；`-o -` 会写到标准输出 |
| `cookies import <chrome\|edge>`  | 从正在运行的浏览器复制 Cookie，以及其打开的标签页中网站的 localStorage 和 IndexedDB，让面板在你于浏览器中登录过的网站上也处于登录状态；`--domain` 只复制某个网站的（[步骤](./panes.md#signing-in-with-your-browsers-cookies)） |
| `wait-message [--json]`         | 等待页面用 [`window.kero.send`](./panes.md#sending-to-an-agent-from-a-page) 发送的下一条消息，并附上页面信息打印出来；`--json` 打印其 `id`、`from`、`label`、`url`、`title`、`text` 和 `at` |
| `pick`                          | 打开选取元素模式，让用户选取元素并为每个写备注；选取结果作为消息发给页面的智能体（[详情](./panes.md#为智能体选取元素)） |
| `console [--errors] [--clear]`  | 打印页面用 `console.*` 输出的内容、抛出的错误、无人捕获的 Promise，以及浏览器报告的内容（如 CORS 错误和加载失败），每条附文件和行号；`--errors` 只打印错误，`--clear` 在打印后清空日志 |
| `network [--failed] [--url text] [--clear]` | 打印页面的请求，每个一行：`#id`、方法、状态、类型、大小、耗时和 URL，以及失败原因（`net::ERR_…`、`CORS …`、`blocked: …`）；`--failed` 只打印失败或返回 4xx、5xx 的，`--url` 只打印 URL 含该文本的 |
| `network body <id> [-o file]`   | 打印列表中某个响应的正文，或保存到文件 |
| `emulate device <name>`         | 像手机或平板那样显示页面：视口、缩放、触摸和 user agent；`emulate device` 列出可选设备（`iphone-se`、`iphone-15`、`iphone-15-pro-max`、`pixel-8`、`galaxy-s24`、`ipad`、`ipad-pro`） |
| `emulate viewport <w>x<h>`      | 按该尺寸布局页面；`--scale` 设定每个 CSS 像素对应的设备像素，`--mobile` 像手机那样布局，`--touch` 让页面接收触摸；`none` 恢复 |
| `emulate user-agent <ua…>`      | 发送另一个 user agent；`none` 恢复 |
| `emulate network <preset>`      | 让网络变慢（`slow-3g`、`3g`、`4g`）、断网（`offline`），或恢复（`none`） |
| `emulate [reset]`               | 打印页面当前的显示方式；`reset` 全部恢复原样 |

除非用 `--timeout` 指定，`wait-message` 会一直等待；没有收到消息时以 `timed out` 失败：智能体应以短于其 shell 工具自身超时的时间重新等待。它不需要 Remote Control。

选择器是 CSS 选择器，在 shell 中记得加引号。需要元素的命令会等它出现，`--timeout`（默认 `10s`）设定这些命令以及页面加载最多等多久。`type` 和 `eval` 接受 `-`，表示从标准输入读取文本。

点击和输入发送的是真实鼠标和键盘会产生的事件，所以 React 等框架构建的页面也能收到。截图需要窗格显示在屏幕上：先用 `kero focus` 显示它。其余命令对任何浏览器窗格都有效，无论是否显示。

`console` 和 `network` 保留窗格的页面创建以来（包括首次加载）的内容，各自最多最近 1,000 条；`--clear` 清空它们，这样下次读取时只显示重新加载或点击所引起的内容。`--json` 打印它们的字段，以及 `emulate` 的状态。`emulate` 的设置在窗格打开期间一直有效，重新加载和前往其他页面后也是。`network` 和 `emulate` 使用 DevTools Protocol，只有 Windows 上的浏览器窗格才有；在 macOS 和 Linux 上，`console` 只包含当前文档输出的内容。

和 `kero do` 一样，`kero browser` 需要开启 **设置 › 智能体** 中的 **远程控制**，并且 app 处于打开状态。

在 Windows 上，Playwright、chrome-devtools-mcp 等基于 DevTools 协议的工具也能通过 **DevTools 端口** 打开的端口操作窗格（[步骤](./panes.md#driving-panes-with-devtools-tools)）。`kero browser devtools` 打印端口，以及每个窗格在其中的页面（目标 id、URL 和标题）；`--json` 以 JSON 打印。它不需要远程控制。

## 过去的对话

```sh
kero history                     # 窗格中运行过的智能体对话，最近运行的在前
kero history -p kero -n 50       # 某个项目的对话，最多 50 个
kero history login --json        # 与 “login” 匹配的对话，输出 JSON
kero resume 13b9                 # 恢复一个对话：用 ID、ID 的前缀或名字
kero resume fix-login -f         # 恢复并在 app 中显示
```

`kero resume` 会在对话的目录和项目中启动一个恢复该对话的会话，并打印会话的 ID。如果某个窗格仍在运行这个对话，则返回那个会话。参见 [过去的对话](./sessions.md#past-conversations)。

## 状态

```sh
kero status api --json                    # 状态、程序、目录
kero report working "Running migrations"  # 报告自己的状态
kero notify "Need your input"             # 将此窗格标记为需要你处理
```

各状态的含义见 [状态与通知](./status.md)。

## 改动

```sh
kero diff                                 # 窗格中未提交的改动，格式同 git diff
kero diff --turn                          # 它的智能体在上一轮中改了什么
kero diff review --branch                 # 自默认分支以来的改动
```

见 [改动](./changes.md)。

## 草稿审查

审查拉取请求的智能体把评论写进[草稿审查](./changes.md#draft-reviews)，由你在 Kero 中发布。行号是拉取请求头部中文件的行号（加 `--base` 时为基础分支的行号），并且必须在拉取请求的差异中：

```sh
kero pr comment src/parse.go:42 "This drops the last token."
kero pr comment src/parse.go:40-44 -      # 正文从标准输入读取
kero pr comment src/api.go "Split this file?"   # 整个文件
kero pr summary "Two bugs in the parser."
kero pr show                              # 草稿，以及每条评论的 id
kero pr edit 3 "Clearer text"
kero pr delete 3
```

在从 Kero 开始的审查中，窗格知道它的拉取请求；在其他地方，审查有草稿后用 `--pr <编号>` 指定；在拉取请求上点 **开始草稿** 即可开始一个草稿。

## 审查导览

智能体用 `kero pr guide` 编写拉取请求的[审查导览](./changes.md#review-guides)（导览标签页中的讲解）。站点按添加顺序排列，`--at <n>` 可放在第 n 站；行必须包含拉取请求差异的一部分，只给路径则是整个文件。导览不会发送到 GitHub：

```sh
kero pr guide summary "Moves retries onto a queue."
kero pr guide stop src/queue.go:10-42 --title "The new queue" "Why it exists."
kero pr guide stop src/api.go:88 --title "Who enqueues" --closely   # Look closely
kero pr guide stop testdata/golden.txt --title "Snapshots"          # the whole file
kero pr guide show                        # the guide, each stop with its id
kero pr guide edit 2 --title "Clearer"    # or move 2 --at 1, delete 2
kero pr guide clear                       # start the tour again
```

在从 Kero 开始的导览中，窗格知道它的拉取请求；在其他地方，用 `--pr <编号>` 指定。

Kero 为会话的智能体关注的会话[拉取请求](./changes.md#会话的拉取请求)：

```sh
kero pr link                 # 会话分支上打开的拉取请求
kero pr link 123             # 或它的编号、URL
kero pr status --json        # 状态：检查、失败的检查、冲突
kero pr unlink
```

以登录 `gh` 的用户身份操作 GitHub 上的拉取请求：给出它的编号，或省略以使用会话关联的那个。合并时如未指定方式，就用仓库的默认方式（**仓库设置**中设置的，否则是仓库允许的 squash、merge、rebase 中的第一个）：

```sh
kero pr state 123                          # 是否打开、是否草稿、能否合并、可用哪些方式
kero pr ready 123                          # 或 draft、close、reopen
kero pr update-branch 123 --rebase         # 让它的分支跟上基础分支
kero pr merge 123 --squash --delete-branch # 或 --merge、--rebase；--head <sha> 只在该提交处合并
kero pr merge 123 --auto                   # 满足要求后自动合并；--disable-auto 取消
kero pr delete-branch 123                  # 在关闭或合并之后
kero pr checkout 123                       # 在这里检出它的分支，工作树有改动时拒绝
kero pr checkout 123 --worktree -- claude  # 在专属工作树中新建会话，并输出其 id
```

如果已有工作树包含该拉取请求，就直接使用它：`kero pr checkout` 会告诉你位置，带 `--worktree` 时输出在那里工作的会话，没有就在那里启动一个。

## 配置与设置

```sh
kero config path                 # 配置文件所在位置
kero config show                 # 每个配置项及其值
kero config show --defaults      # 每个配置项的默认值
kero config edit                 # 在 $EDITOR 中打开它
kero config check                # 报告未知的配置项、无效的值和agents.toml 的问题
kero projects                    # 按边栏的顺序列出项目
kero projects move <project> top # 移动：top、bottom、up、down 或位置（1 为顶部）
kero skill setup                 # 教编程智能体使用 kero
kero daemon status|stop|restart  # 管理 kerod
kero daemon install-service      # 让 kerod 随系统启动（systemd、launchd）
kero daemon serve                # 在前台运行 kerod，供服务管理器使用
kero clients                     # 查看谁连接着 kerod
```

## 其他机器

在命令前加 `-H <host>`，或设置 `$KERO_HOST`，可以让操作会话的命令指向另一台机器上的 kerod：可以是你添加过的主机，也可以是 `me@devbox`、`wsl://Ubuntu` 这样的地址。见 [远程主机](./remote.md)。

```sh
kero hosts                       # 你添加过的主机
kero hosts add devbox me@devbox  # 添加一台，app 中也会显示
kero hosts check devbox          # 尝试连接，并报告结果
kero hosts install devbox        # 通过 SSH 在那里安装 Kero
kero ls --all                    # 本机和每台主机上的会话
kero -H devbox ls
kero -H devbox new -n review -- claude
kero -H devbox daemon update     # 将那里的 Kero 更新到本机的版本
```

在窗格中，`kero` 总是与窗格所在机器的 kerod 通信，无论 app 在哪里。

`kero ls --all` 同时询问每台主机，把各自的会话列在 `-H` 所用的名称下（本机为 `local`）。连不上的主机会在会话的位置说明原因。加上 `--json` 时，输出 `{host, addr, error, sessions}` 的列表。

<a id="managing-kero"></a>
## 管理 Kero

边栏、其菜单和设置能做的事，用下面的命令也能在 shell 中做到。[Ama](./agents.md#助手代理-ama) 也使用它们：

```sh
kero ls -p api --status waiting                # 按状态列出项目的会话
kero projects add ~/code/api -n api            # 打印其 id
kero projects rename api backend
kero projects remove backend --force           # 其会话也会关闭
kero move <session> <project>
kero close --all -p api --status idle,exited --dry-run   # 查看将关闭哪些
kero close --all -p api --status idle,exited
kero worktree create fix-login -p api -- claude "fix the login"
kero worktree remove fix-login -p api --branch # 也删除分支
kero worktree cleanup --dry-run                # 自动清理会删除什么
kero worktree prune -p api
kero pr list -p api
kero pr review 123 -p api                      # 与评审标签页的启动方式相同
kero config get default-agent
kero config set notify-on-done false           # 与设置的写法相同
```

带 `--all`、`-p` 或 `--status` 的 `kero close` 从不关闭 Ama 的会话，也不关闭运行它的会话。`kero worktree remove` 像历史标签页那样删除工作树：Kero 为某个会话创建的工作树会连同该会话一起关闭，其他会话正在使用的则保留。`kero pr review` 需要应用已打开，且 Ama 以外需要远程控制。不带值的 `kero config set` 恢复默认值；列表设置每个值一个参数，不给值则清空。

## 执行 app 操作

`kero do` 可以执行 app 的任意操作，与菜单中的操作相同。它需要 **远程控制**，该选项在 **设置 › 智能体** 中默认开启。

```sh
kero actions                     # 列出所有操作
kero do pane.split.right
kero do pane.browser url=localhost:3000
kero do sidebar.toggle
kero do files.show path=src/main.go
```

相对路径的 `path=` 或 `dir=` 是相对于运行 `kero do` 时所在的文件夹。

`kero do pane.preview` 按渲染后的样子显示编辑器窗格的 Markdown 文件，或回到文本：`on`、`off`，都不写则切换。它作用于 `pane=` 指定的窗格，否则作用于会话中 Markdown 文件的编辑器窗格。它和 `kero split --preview` 一样设置 kerod 保存的窗格状态，因此既不需要远程控制，也不需要打开应用。

```sh
kero do pane.preview on
kero do pane.preview off pane=4f2a
```

## 退出码

| 退出码 | 含义                           |
| ------ | ------------------------------ |
| `0`    | 成功                           |
| `1`    | 失败                           |
| `2`    | 用法错误                       |
| `3`    | 目标不存在，或有歧义           |
| `4`    | 无法连接 kerod                 |

等待程序的命令，即 `kero wait --exit` 和 `--wait`，改为以该程序的退出码退出；当窗格的 shell 以错误退出时，`kero wait` 也是如此。等待超时则以 `1` 退出。

给 `ls`、`status` 和 `projects` 加上 `--json`，可得到便于程序处理的输出。
