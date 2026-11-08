---
title: 窗格
description: 把会话拆分成多个窗格，并排运行开发服务器、测试或另一个智能体。
---

# 窗格

拆分会话，让多个程序并排运行：左边是智能体，右边是它的开发服务器，下方是测试监视器。

| 操作               | 快捷键                                            |
| ------------------ | ------------------------------------------------- |
| 向右拆分           | <kbd>⌘</kbd> <kbd>D</kbd>                         |
| 向下拆分           | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd>            |
| 放大窗格           | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd>            |
| 下一个 / 上一个窗格 | <kbd>⌘</kbd> <kbd>]</kbd> / <kbd>[</kbd>        |
| 在窗格间移动       | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> <kbd>→</kbd> <kbd>↑</kbd> <kbd>↓</kbd> |
| 重启窗格           | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd>            |
| 关闭窗格           | <kbd>⌘</kbd> <kbd>W</kbd>                         |

**放大**会让一个窗格占满整个会话；再按一次即可恢复其他窗格。

在边栏中，拆分过的会话会在名称后显示它有几个窗格。点击这个数字，窗格会列在该行下方，每个显示其程序、名称、与会话不同时的分支或文件夹，以及状态，从而看出是哪个窗格需要你。点击其中的窗格即可前往，右键点击可以放大或关闭它。再次点击数字即可收起。

窗格中的程序退出后，窗格会保留下来，方便你查看输出。**重启窗格**会在同一目录下重新启动它的 shell。

要让智能体看到终端输出的内容，例如失败测试的输出，选中文本，右键点击并选择 **发送到会话…**，再选择智能体的会话。智能体会以代码块的形式收到这段文本，并附上来源窗格及其中运行的内容（Claude Code 等智能体、某个程序，或 pwsh 等 shell）；它回到提示符时，文本就会输入进去。

## 通过命令行

`kero split` 在另一个窗格旁启动新窗格，并打印其 id：

```sh
kero split --down -- npm run dev
kero split --right --ratio 0.3 -- go test ./... -count=1
```

| 参数                                  | 含义                                           |
| ------------------------------------- | ---------------------------------------------- |
| `--right`（默认）、`--down`、`--left`、`--up` | 新窗格的位置                           |
| `-r`, `--ratio <0..1>`                | 新窗格所占的空间比例（默认 0.5）               |
| `-d`, `--dir <dir>`                   | 在另一个目录中启动                             |
| `-w`, `--wait`                        | 等待其命令结束，并以该命令的状态码退出         |
| `-u`, `--url <url>`                   | 打开浏览器显示该页面，而不是 shell；本地文件的路径则显示该文件 |
| `--file <path>`                       | 打开该文件的编辑器，而不是 shell               |
| `--preview`                           | 对 `--file` 的 Markdown 文件，按渲染后的样子显示，而不是文本 |

在 Kero 中运行时，`kero split` 会拆分它所在的窗格。在其他地方运行时，需要指定目标：`kero split my-session --down`。

会话中已有窗格显示的文件或本地页面不会再次拆分：`kero split` 会聚焦那个窗格（带 `--preview` 时切换为预览），输出它的 ID，并在 stderr 中说明。重写了计划的代理再次展示时，会在原处找到它。

## 编辑器窗格

编辑器窗格在终端旁显示一个文件，按其语言高亮，可阅读和编辑：<kbd>⌘</kbd> <kbd>S</kbd>（<kbd>Ctrl</kbd> <kbd>S</kbd>）保存，与[查看器](./changes.md#editing)一样，只在文件仍是读取时的样子时才写入。它像其他窗格一样可以拆分、放大、移动和关闭，Kero 重启后会回到同一个文件。有未保存的编辑时关闭它，会先询问你。

从文件标签页打开：右键点击文件并选择**在窗格中打开**，或在查看器显示该文件时点击其标题栏中的**在窗格中打开**。也可以把文件拖到窗格上：放在边缘附近，会在窗格该侧旁边打开；放在编辑器窗格中间，会替换其中的文件（除非它有未保存的编辑）。从文件管理器拖来的文件和文件夹也这样打开，文件夹会在该位置打开一个终端；拖到终端上时，只有靠近其左、右或上边缘才会打开，其余位置会粘贴它们的路径。在 shell 中或由智能体：

```sh
kero split --file src/main.go
kero new --file src/main.go     # 在独立会话中
```

如果目标文件尚不存在，但其所在文件夹已存在，它会以空文件的形式打开，并标记为 **新文件**；保存时会创建该文件。

右键点击窗格的拆分按钮（向右或向下）并选择**新建文件**，会在窗格所在文件夹中打开一个空的 `untitled.txt`。在你输入之前，其标题栏中的**打开文件…**可改为打开你选择的文件。

右键点击窗格顶部的文件名，可以复制它的路径（完整路径或相对于会话文件夹的路径）、在编辑器或其他应用中打开、在文件管理器中显示，或查看它的历史。

它的标题栏和查看器一样有文件的按钮：保存、长行换行和在编辑器中打开；有未保存的编辑时会显示一个圆点。窗格太窄放不下时，这些按钮会收进**文件操作**菜单。Markdown 文件还有 **预览 Markdown**，按渲染后的样子显示它，包括旁边的图片；再次点击即可回到编辑文本。**缩小文字**和**放大文字**会独立于终端，缩放所有文件及其 Markdown 预览的文字；文件获得焦点时，<kbd>⌘</kbd> <kbd>=</kbd>、<kbd>⌘</kbd> <kbd>-</kbd> 和 <kbd>⌘</kbd> <kbd>0</kbd>（Windows 和 Linux 上为 <kbd>Ctrl</kbd>）效果相同。

**窗格 › 切换 Markdown 预览** 对你所在的窗格做同样的事。Kero 重启后，窗格仍保持预览。代理可以以预览打开它，或切换预览：

```sh
kero split --file plans/plan.md --preview
kero do pane.preview off
```

窗格会跟随它的文件：当代理等其他程序写入该文件时，窗格显示写入的内容；但如果你有未保存的编辑，它们会保留，保存时会询问是否覆盖。

## 浏览器窗格

浏览器窗格在终端旁显示网页：智能体正在开发的开发服务器、它在读的文档、它提交的拉取请求。它和其他窗格一样可以拆分、放大、移动和关闭，Kero 重启后也会回到同一页面。

| 操作             | 快捷键                                 |
| ---------------- | -------------------------------------- |
| 新建浏览器窗格   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> |
| 打开地址         | <kbd>⌘</kbd> <kbd>L</kbd>              |
| 选取元素         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> |

右键点击窗格的拆分按钮并选择**浏览器**，会在该窗格旁打开一个。在地址栏中输入网址，例如 `localhost:3000` 或 `github.com`，或者输入要搜索的词。在命令面板（<kbd>⌘</kbd> <kbd>P</kbd>）中输入的网址也会在新的浏览器窗格中打开。<kbd>⌘</kbd> <kbd>F</kbd> 可在页面中查找文本。

在 shell 中或由智能体用 `--url` 打开：

```sh
kero split --url localhost:5173
kero new --url https://github.com/egoist/kero/pulls
kero split --url .backlog/pages/brief.html
```

本机文件的路径（相对于运行 `kero split` 的文件夹）或 `file:///` URL 会显示本地页面：旁边的图片和样式表从磁盘加载，CDN 上的脚本从网络加载。文件在磁盘上变化时，页面会重新加载。本地页面无法以模块（`<script type="module">`）方式加载或 fetch 同一文件夹中的脚本：浏览器只允许从服务器这样做。

之后智能体就能像你一样通过 `kero browser` 使用这个页面：点击、输入、读取文本、等待加载以及截图。参见[命令行](./cli.md#browsing)。

```sh
kero browser type 'input[name=q]' "kero" --enter
kero browser text main
```

<a id="sending-to-an-agent-from-a-page"></a>
### 从页面发送给智能体

浏览器窗格中的本地页面（`file:`）可以给智能体发送消息，例如你在智能体展示的决策简报（decision brief）上做的选择。页面调用 `window.kero.send(message, options)`；Kero 会在智能体回到提示符时，把消息输入到用 `kero split --url` 打开（或再次显示）该页面的智能体窗格中；如果智能体正用 [`kero browser wait-message`](./cli.md#browsing) 等待，就直接交给它。对于不是智能体打开的页面，Kero 会像 **发送到会话…** 一样询问发给哪个智能体，之后都发到那里。

每条消息都会告诉智能体它来自哪里：`Kero: message from database pick, from the page "Pick and send" (file:///…/send-to-agent.html) in browser pane 3f2a:`，后面是消息文本。`send` 返回的 Promise 会在智能体收到消息后完成，页面就能显示"已发送"；如果智能体的窗格先关闭，则会 reject。

| 选项      | 作用 |
| --------- | ---- |
| `label`   | 消息是什么，放在页面标题和 URL 之前 |
| `mode`    | `"push"` 总是输入到提示符；`"wait"` 只留给 `kero browser wait-message`；不设置时，交给正在等待的智能体，否则输入到提示符 |
| `onState` | 在 Promise 完成之前收到 `"queued"`（智能体正在工作：回到提示符时收到）或 `"waiting"`（留给 `wait-message`） |

不是字符串的消息会以 JSON 发送。`window.kero` 只存在于 Kero 中，所以页面可以据此判断能否发送，不能发送时把消息复制到剪贴板：

```html
<!doctype html>
<meta charset="utf-8">
<title>Pick and send</title>
<h1>Which database?</h1>
<label><input type="radio" name="d1" value="A" checked> A: SQLite</label>
<label><input type="radio" name="d1" value="B"> B: Postgres</label>
<p><button id="send">Send</button> <span id="status"></span></p>
<script>
const send = document.getElementById("send");
const status = document.getElementById("status");
send.onclick = async () => {
  const text = "D1: " + document.querySelector('input[name="d1"]:checked').value;
  if (!window.kero) {
    await navigator.clipboard.writeText(text);
    status.textContent = "Copied: paste it to the agent";
    return;
  }
  status.textContent = "Sending…";
  try {
    await window.kero.send(text, {
      label: "database pick",
      onState: (state) => {
        status.textContent = state === "queued" ? "Queued: the agent is working" : "Waiting for the agent";
      },
    });
    status.textContent = "Sent";
  } catch (err) {
    status.textContent = "Not sent: " + err.message;
  }
};
</script>
```

`window` 上的 `kero:delivery` 事件也会告知每条消息的投递情况，带有 `id`、`state`（`delivered` 或 `failed`）和 `error`。

### 为智能体选取元素

要告诉智能体页面上哪里需要修改，打开**选取元素**：窗格工具栏中方框加指针的按钮，<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd>（Windows 和 Linux 上为 <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>C</kbd>），或窗格菜单。点击一个元素，在弹出的备注中写下你想要的修改，然后继续选取；书写时移动指针，备注保持不变。**Parent** 会改选它外层的元素：把指针放在按钮上，会用虚线框出将选中的元素。**Send** 或 <kbd>Ctrl</kbd> <kbd>Enter</kbd> 一次全部发送，<kbd>Esc</kbd> 停止。

选取的元素和页面自己的消息发往同一处：发给用 `kero split --url` 打开该页面的智能体，在它回到提示符时输入；正在等待该页面消息的智能体会直接收到。对于不是由智能体打开的页面，Kero 会询问一次发给哪个智能体并记住；不选择就关闭时，Kero 会复制它们，方便你粘贴。

CSS 选择器很难把智能体带到源码，所以每个元素都附带能带它找到源码的信息，从强到弱：

1. **在源码中的位置**（页面能提供时）：Svelte 和 React 18 开发构建给出的文件和行，React 19 的调用栈经源码映射得到的行，Vue 组件的文件及其中该标签所在的行，以及 `data-v-inspector` 等检查器插件添加的属性。Kero 会用文件核对行号，标签不在该行时加以修正。对于本地文件的页面，则是元素起始标签在文件中的行。两者都没有时，消息会写明没有，智能体便直接使用提示。
2. **搜索提示**：源码中很可能原样写着、可以 grep 的字符串：class 属性、id、测试 id、标签、name、链接、占位符，以及元素自身文本中的一两个短语（4 到 8 个词，在词的边界结束，不含 ’、— 等源码可能写成实体的字符）；页面上只有它拥有的会加标记。
3. **在页面上的位置**：上方的地标和标题、它是同类中的第几个（“在 `<main>` 中，标题 "Pricing" 下方，3 个 `div.card` 中的第 2 个”）、父元素和兄弟元素，以及截短的 HTML。其中的 `<script>`、`<style>`、`<svg>`、`<template>` 和 `<noscript>` 保留标签和属性，内容只显示大小，例如 `/* 1.2 KB omitted */`。
4. 选择器和 DOM 路径、页面 URL 和视口、主要的计算样式，以及框出该元素的截图。

截图是临时文件夹中 `kero-picks` 文件夹里的 PNG 文件（一天后删除），消息中会写出其路径，终端中的智能体可以按路径打开。选取栏的 **Screenshots** 会拍两种：**Elements** 为每个元素拍一张带边距、加框的截图（已滚出视野的元素会先滚回视野再拍，之后滚回原处），**Page** 拍下你看到的页面，每个元素都加框，并按消息中的编号标注。两者默认都打开；你关掉的选项，Kero 会在下次选取时记住。

组件信息只来自开发构建：生产构建会发送提示及其后的内容。

<a id="blocking-ads"></a>
### 拦截广告

浏览器窗格使用过滤列表拦截广告和跟踪器，默认为 [EasyList](https://easylist.to) 和 EasyPrivacy，你也可以另选。Kero 会在首次打开浏览器窗格时下载这些列表，并每周更新。在列表就绪前打开的页面，会从列表就绪后的下一次加载起生效。

在**设置 › 浏览器**的**拦截广告**下选择列表：其中还有针对 Cookie 提示、社交按钮、页面干扰元素和中文网站广告的列表。其他网址的列表可以在[配置](./configuration.md#browser)中通过 `browser-filter-list` 添加。列表越多，隐藏的内容越多，偶尔也会导致页面出错。

Kero 能拦截的，就是 WebKit 内容拦截器所能拦截的：发往广告和跟踪服务器的请求，以及列表要隐藏的元素。需要拦截器自身脚本才能实现的过滤规则，例如按文本隐藏元素或改写页面加载内容的规则，不会生效。列表所拦截域名上的页面无法在浏览器窗格中打开。

如需不拦截地浏览，可在**设置 › 浏览器**中关闭**拦截广告**，或在[配置](./configuration.md#browser)中设置 `browser-block-ads = false`。已打开的页面会保留已加载的内容，直到重新加载。

<a id="signing-in-with-your-browsers-cookies"></a>
### 用浏览器的 Cookie 登录

浏览器面板有自己的 Cookie，所以你在 Chrome 或 Edge 中已登录的网站，在面板中会要求你重新登录。与其逐个登录，不如把浏览器的 Cookie 交给面板：

1. 在 Chrome（144 或更高版本）或 Edge 中打开 `chrome://inspect/#remote-debugging`（Edge 中为 `edge://inspect/#remote-debugging`），开启远程调试。它会一直有效，直到浏览器退出。
2. 在**设置 › 浏览器**的**登录状态**下，点击浏览器旁的**导入**，或运行 `kero browser cookies import chrome`（或 `edge`）。
3. 浏览器会询问是否允许连接：允许即可。对话框可能出现在浏览器的任一窗口中，包括已安装应用（PWA）的窗口，例如你从某个网站添加的桌面应用；普通窗口中没有时，请到那里找。两种情况下导入的结果相同：已安装的应用也运行在浏览器及其配置文件中。

Kero 会复制浏览器视为默认的配置文件中的 Cookie，让面板在你于浏览器中登录过的网站上也处于登录状态。它不会改动浏览器：读取 Cookie 后即断开连接。

许多网页应用把登录状态保存在网站的 localStorage 或 IndexedDB 中，而不是 Cookie 里。浏览器只把网站的存储交给该网站的页面，所以 Kero 会复制浏览器中打开的标签页所属网站的存储：导入前先在浏览器中打开该应用，导入后重新加载它的面板。`kero browser cookies import chrome --domain app.example.com` 只复制一个网站的存储；若没有打开该网站的标签页，会在读取期间打开一个不可见的标签页。网站的数据库会替换面板中同名的数据库；超过 8 MB 的数据库，以及浏览器自己保管、页面无法复制的值（例如网站的加密密钥）不会导入，Kero 会告诉你哪些被略过。登录过期后再导入一次即可。Kero 重启后面板仍保留这些 Cookie，所有 `kero browser` 命令照常作用于页面。

Chrome 和 Edge 不允许其他程序打开你的配置文件或读取其 Cookie 文件，并会忽略针对它的 `--remote-debugging-port`：`chrome://inspect` 中的开关是唯一的途径，而且每次都会征求你的同意。Cookie 就是登录凭据：会话中的智能体和你一样，可以使用这些页面和 `kero browser cookies save`。

<a id="driving-panes-with-devtools-tools"></a>
### 用 DevTools 工具操作窗格

在 Windows 上，浏览器窗格可以打开一个 DevTools 端口（即 Chrome 用 `--remote-debugging-port` 打开的那种），供基于 DevTools 协议的工具使用：[Playwright](https://playwright.dev) 的 `connectOverCDP`、[chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp)、Puppeteer 和 Lighthouse。在**设置 › 浏览器**中打开 **DevTools 端口**，或在[配置](./configuration.md#browser)中设置 `browser-devtools = true`，然后退出 Kero 并重新启动：窗格的浏览器在启动时打开端口。

每个窗格都是同一个浏览器的一个页面，所以所有窗格共用一个端口，除非用 `browser-devtools-port` 另行指定，否则为 `http://127.0.0.1:9222`。工具按 URL 或标题选择窗格。`kero browser devtools` 打印端口以及每个窗格在其中的页面：

```sh
$ kero browser devtools
http://127.0.0.1:9222
3f2a	8C1D…	http://localhost:5173/	Vite App
9b41	E07A…	https://github.com/egoist/kero	GitHub - egoist/kero
```

`--json` 打印 `endpoint`、`port`、`browser`（浏览器的 WebSocket URL），以及每个窗格的 `pane`、`session`、`url`、`title`、`target` 和 `webSocketDebuggerUrl`。还没显示过页面的窗格没有目标：先显示一次。

用 Playwright 时，连接端口，按 URL 取得窗格的页面：

```js
import { chromium } from "playwright";

const browser = await chromium.connectOverCDP("http://127.0.0.1:9222");
const page = browser.contexts()[0].pages().find((p) => p.url().startsWith("http://localhost:5173"));
await page.getByRole("button", { name: "Add" }).click();
await browser.close(); // 只断开连接，窗格保持打开
```

对于 chrome-devtools-mcp，把端口作为 `--browser-url` 交给智能体（用 Claude Code，或在其他智能体的 MCP 设置中）：

```sh
claude mcp add chrome-devtools -- npx chrome-devtools-mcp@latest --browser-url=http://127.0.0.1:9222
```

```json
{ "mcpServers": { "chrome-devtools": { "command": "npx", "args": ["chrome-devtools-mcp@latest", "--browser-url=http://127.0.0.1:9222"] } } }
```

智能体随后列出窗格的页面，并按 URL 或标题选择一个。请在 Kero 中打开窗格，例如用 `kero split --url`：工具自己打开的新标签页或窗口不是窗格，WebView2 也可能拒绝它们。

端口只监听 `127.0.0.1`，但打开后，这台电脑上的任何程序都能不经询问地使用窗格的页面及其登录状态：只在需要时打开。端口仅限 Windows：macOS 和 Linux 上系统的 Web 视图不会打开端口，在那里请用 `kero browser` 操作窗格。
