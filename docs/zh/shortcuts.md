---
title: 键盘快捷键
description: Kero 的默认键盘快捷键，以及用于重新绑定的操作 id。
---

# 键盘快捷键

以下任意快捷键都可以在 **设置 › 键盘** 中重新绑定，也可以在[配置文件](./configuration.md#keyboard)中用 `keybind` 修改。

在 Windows 和 Linux 上，<kbd>Ctrl</kbd> 加字母的组合归 shell 使用（<kbd>Ctrl</kbd> <kbd>D</kbd>、<kbd>Ctrl</kbd> <kbd>R</kbd>、<kbd>Ctrl</kbd> <kbd>W</kbd>……），因此 Kero 的快捷键会像 Windows Terminal 一样额外加上 <kbd>Shift</kbd>，或改用 <kbd>Alt</kbd>。短横线表示该操作在对应平台上没有快捷键。

## Kero

| 操作                         | macOS                                  | Windows 和 Linux                              | 操作 id          |
| ---------------------------- | -------------------------------------- | --------------------------------------------- | ---------------- |
| 设置…                        | <kbd>⌘</kbd> <kbd>,</kbd>              | <kbd>Ctrl</kbd> <kbd>,</kbd>                  | `settings.open`  |
| 退出（会话继续运行）         | <kbd>⌘</kbd> <kbd>Q</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Q</kbd> |                  |
| 退出并结束所有会话…          | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>Q</kbd> | –                                             | `app.quitAll`    |
| 安装命令行工具…              | –                                      | –                                             | `cli.install`    |

`kero actions` 列出 `kero do` 能执行的操作，以及它们在这台电脑上的快捷键。这里的其他操作，如 `app.quitAll`、`session.rename` 或 `find.open`，可以绑定快捷键，但 `kero do` 不会执行它们（[执行 app 操作](./cli.md#running-app-actions)）。

## 编辑

| 操作          | macOS                                  | Windows 和 Linux                              | 操作 id         |
| ------------- | -------------------------------------- | --------------------------------------------- | --------------- |
| 查找…         | <kbd>⌘</kbd> <kbd>F</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>F</kbd> | `find.open`     |
| 查找下一个    | <kbd>⌘</kbd> <kbd>G</kbd>              | –                                             | `find.next`     |
| 查找上一个    | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>G</kbd> | –                                             | `find.previous` |

查找会在你所在的窗格中搜索。在查看器中，它搜索查看器中的文件或 diff；在右侧边栏的文件树中，它会打开[搜索](./files.md#search)。

## 会话

| 操作                        | macOS                                  | Windows 和 Linux                              | 操作 id             |
| --------------------------- | -------------------------------------- | --------------------------------------------- | ------------------- |
| 新建会话                    | <kbd>⌘</kbd> <kbd>T</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>T</kbd> | `session.new`       |
| 在主目录新建会话            | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>T</kbd> | –                                             | `session.newHome`   |
| 在工作树中新建会话…         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd> | –                                             | `session.newWorktree` |
| 打开项目…                   | <kbd>⌘</kbd> <kbd>O</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>O</kbd> | `project.open`      |
| 前往会话…                   | <kbd>⌘</kbd> <kbd>P</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>P</kbd> | `palette.open`      |
| 前往文件…                   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>N</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>N</kbd> | `files.goto`        |
| 恢复对话…                   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>O</kbd> | –                                             | `conversation.resume` |
| 搜索对话…                   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>F</kbd> | –                                             | `conversation.search` |
| 跳到需要你的会话            | <kbd>⌘</kbd> <kbd>J</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>J</kbd> | `session.attention` |
| 下一个会话                  | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>]</kbd> | <kbd>Ctrl</kbd> <kbd>PgDn</kbd>               | `session.next`      |
| 上一个会话                  | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>[</kbd> | <kbd>Ctrl</kbd> <kbd>PgUp</kbd>               | `session.previous`  |
| 重命名会话                  | <kbd>⌘</kbd> <kbd>R</kbd>              | –                                             | `session.rename`    |
| 重启窗格                    | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>R</kbd> | `pane.restart`      |
| 关闭                        | <kbd>⌘</kbd> <kbd>W</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>W</kbd> | `pane.close`        |
| 关闭会话                    | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>W</kbd> | –                                             | `session.close`     |

<kbd>⌘</kbd> <kbd>1</kbd>–<kbd>9</kbd>（Windows 和 Linux 上为 <kbd>Ctrl</kbd> <kbd>1</kbd>–<kbd>9</kbd>）跳到边栏中对应的会话，<kbd>Ctrl</kbd> <kbd>Tab</kbd> 跳到下一个会话。

## 窗格

| 操作          | macOS                                  | Windows 和 Linux                                  | 操作 id            |
| ------------- | -------------------------------------- | ------------------------------------------------- | ------------------ |
| 向右拆分      | <kbd>⌘</kbd> <kbd>D</kbd>              | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>=</kbd>      | `pane.split.right` |
| 向下拆分      | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>-</kbd>      | `pane.split.down`  |
| 新建浏览器面板 | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>B</kbd>     | `pane.browser`     |
| 打开位置      | <kbd>⌘</kbd> <kbd>L</kbd>              | –                                                 | `browser.address`  |
| 选取元素 | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>C</kbd> | `browser.pick` |
| 放大窗格      | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Enter</kbd> | `pane.zoom`        |
| 切换 Markdown 预览 | –                                 | –                                                 | `pane.preview`     |
| 下一个窗格    | <kbd>⌘</kbd> <kbd>]</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>]</kbd>     | `pane.next`        |
| 上一个窗格    | <kbd>⌘</kbd> <kbd>[</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>[</kbd>     | `pane.previous`    |
| 左侧窗格      | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> | <kbd>Alt</kbd> <kbd>←</kbd>                       | `pane.left`        |
| 右侧窗格      | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>→</kbd> | <kbd>Alt</kbd> <kbd>→</kbd>                       | `pane.right`       |
| 上方窗格      | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↑</kbd> | <kbd>Alt</kbd> <kbd>↑</kbd>                       | `pane.up`          |
| 下方窗格      | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↓</kbd> | <kbd>Alt</kbd> <kbd>↓</kbd>                       | `pane.down`        |

<a id="view"></a>

## 显示

| 操作           | macOS                     | Windows 和 Linux                              | 操作 id          |
| -------------- | ------------------------- | --------------------------------------------- | ---------------- |
| 切换边栏       | <kbd>⌘</kbd> <kbd>B</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>B</kbd> | `sidebar.toggle` |
| 切换右侧边栏   | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>B</kbd> | –                                             | `rightSidebar.toggle` |
| 显示或隐藏改动 | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>G</kbd> | `changes.toggle` |
| 显示或隐藏文件 | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> | `files.toggle`   |
| 显示或隐藏历史 | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>Y</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Y</kbd> | `history.toggle` |
| 显示或隐藏搜索 | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>F</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>S</kbd> | `search.toggle`  |
| 显示或隐藏审查 | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>P</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>K</kbd> | `review.toggle`  |
| 显示改动、文件、历史、搜索 | – | – | `changes.show`、`files.show`、`history.show`、`search.show` |
| 放大           | <kbd>⌘</kbd> <kbd>=</kbd> | <kbd>Ctrl</kbd> <kbd>=</kbd>                  | `text.bigger`    |
| 缩小           | <kbd>⌘</kbd> <kbd>-</kbd> | <kbd>Ctrl</kbd> <kbd>-</kbd>                  | `text.smaller`   |
| 实际大小       | <kbd>⌘</kbd> <kbd>0</kbd> | <kbd>Ctrl</kbd> <kbd>0</kbd>                  | `text.reset`     |
| 切换深色模式 | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>L</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>L</kbd> | `theme.toggle` |
| 快速终端       | –                         | –                                             | `quickTerminal.toggle` |
| Ama            | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>A</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>A</kbd> | `ama.toggle` |

“显示或隐藏”操作会显示右侧边栏的某个标签页，标签页已显示时则隐藏它；“显示”操作只会显示它，`kero do` 为智能体执行的就是这些操作。

<a id="the-command-palette"></a>

## 命令面板

**前往会话…**（<kbd>⌘</kbd> <kbd>P</kbd>，Windows 和 Linux 上为 <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>P</kbd>）就是命令面板：输入文字，可以按名称查找会话，查找要恢复的最近对话，或运行命令：所有菜单操作、设置的某个页面（**设置：Git**）、**重启后台服务**、让电脑保持唤醒、在每个项目和每台主机上新建会话，以及对网址使用的 **在浏览器面板中打开**。无论界面是哪种语言，也都可以用命令的英文名称找到它们。

<a id="quick-terminal"></a>

## 快速终端

快速终端像 Ghostty 的快速终端和 Quake 风格的控制台一样，在任何应用之上从屏幕顶部落下，再按一次快捷键或点击别处时收起。它的快捷键在所有应用中都有效，所以在你于**设置 › 键盘**或[配置文件](./configuration.md#keyboard)中指定之前，它没有快捷键：

```ini
keybind = ctrl+`=quickTerminal.toggle
```

它显示一个专属会话“快速终端”，隐藏时和退出 Kero 后都会继续运行，边栏也会和其他会话一起列出它。shell 退出后，下次按下会启动新的 shell。也可以从**显示 › 快速终端**和菜单栏图标的菜单打开它，或用 `kero do quickTerminal.toggle` 在脚本中切换。

在 Wayland 上，桌面环境通过 XDG 桌面门户绑定该快捷键，可能会请你确认；不支持门户全局快捷键的桌面环境，可以在它们自己的键盘设置中把 `kero do quickTerminal.toggle` 绑定到某个按键。
