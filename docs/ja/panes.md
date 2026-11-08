---
title: ペイン
description: セッションをペインに分割して、開発サーバー、テスト、2 つ目のエージェントを並べて動かします。
---

# ペイン

セッションを分割して、いろいろなものを並べて動かしましょう: 左にエージェント、右にその開発サーバー、下にテストウォッチャー。

| アクション           | ショートカット                                    |
| -------------------- | ------------------------------------------------- |
| 右に分割             | <kbd>⌘</kbd> <kbd>D</kbd>                         |
| 下に分割             | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd>            |
| ペインを拡大         | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd>            |
| 次 / 前のペイン      | <kbd>⌘</kbd> <kbd>]</kbd> / <kbd>[</kbd>          |
| ペイン間を移動       | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> <kbd>→</kbd> <kbd>↑</kbd> <kbd>↓</kbd> |
| ペインを再起動       | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd>            |
| ペインを閉じる       | <kbd>⌘</kbd> <kbd>W</kbd>                         |

**拡大** は 1 つのペインでセッション全体を埋めます。もう一度押すと、ほかのペインが戻ります。

サイドバーでは、分割したセッションの行の名前の後に、ペインの数が表示されます。数をクリックすると、行の下にペインが一覧され、それぞれのプログラム、名前、異なる場合はブランチかフォルダー、状態が表示されるので、どのペインが対応を待っているかがわかります。そこでペインをクリックするとそのペインに移動し、右クリックすると拡大したり閉じたりできます。もう一度数をクリックすると折りたたまれます。

ペインのプログラムが終了しても、出力を読めるようにペインは残ります。**ペインを再起動** は、同じディレクトリでシェルを再び開始します。

失敗したテストの出力など、ターミナルに表示されたものをエージェントに見せるには、テキストを選択して右クリックし、**セッションに送る…** を選んでから、エージェントのセッションを選びます。エージェントは、送り元のペインとそこで動いているもの（Claude Code などのエージェント、プログラム、または pwsh などのシェル）とともに、コードブロックとしてテキストを受け取ります。エージェントがプロンプトに戻った時点で入力されます。

## コマンドラインから

`kero split` は、別のペインの横にペインを開始し、その ID を出力します:

```sh
kero split --down -- npm run dev
kero split --right --ratio 0.3 -- go test ./... -count=1
```

| フラグ                                | 意味                                           |
| ------------------------------------- | ---------------------------------------------- |
| `--right`（デフォルト）、`--down`、`--left`、`--up` | 新しいペインの位置                |
| `-r`, `--ratio <0..1>`                | 新しいペインが占める割合（デフォルト 0.5）     |
| `-d`, `--dir <dir>`                   | 別のディレクトリで開始する                     |
| `-w`, `--wait`                        | コマンドの終了を待ち、その終了ステータスで終了する |
| `-u`, `--url <url>`                   | シェルの代わりに、ページを表示するブラウザを開く。ローカルファイルのパスならそのファイルを表示 |
| `--file <path>`                       | シェルの代わりに、ファイルのエディタを開く     |
| `--preview`                           | `--file` の Markdown ファイルを、テキストではなく表示どおりに見せる |

Kero の中で実行すると、`kero split` は実行したペインを分割します。それ以外の場所では、対象を指定します: `kero split my-session --down`。

セッションのペインがすでに表示しているファイルやローカルページは、もう一度分割しません: `kero split` はそのペインにフォーカスし（`--preview` ならプレビューにして）、その ID を出力し、そのことを stderr に伝えます。書き直した計画を見せるエージェントは、同じ場所でそれを見つけます。

## エディタペイン

エディタペインは、ターミナルの隣にファイルを表示し、言語に合わせてハイライトして、読んだり編集したりできます。<kbd>⌘</kbd> <kbd>S</kbd>（<kbd>Ctrl</kbd> <kbd>S</kbd>）で保存しますが、[ビューア](./changes.md#editing)と同じく、読み込んだときのままのファイルにだけ書き込みます。ほかのペインと同じように分割、ズーム、移動、クローズができ、Kero を再起動しても同じファイルで戻ります。未保存の編集があるまま閉じると、先に確認します。

ファイルタブから開きます: ファイルを右クリックして **ペインで開く** を選ぶか、ビューアがファイルを表示している間にヘッダーの **ペインで開く** をクリックします。ファイルをペインにドラッグすることもできます: 端の近くにドロップすると、その側のペインの隣に開きます。エディタペインの中央にドロップすると、そのファイルを置き換えます（未保存の編集がない場合）。ファイルマネージャーからドラッグしたファイルやフォルダも同じように開きます（フォルダはそこでターミナルとして開きます）。ターミナルでは左・右・上の端の近くだけで、それ以外の場所ではパスを貼り付けます。シェルやエージェントからは:

```sh
kero split --file src/main.go
kero new --file src/main.go     # 独立したセッションで
```

まだ存在しないファイルでも、保存先のフォルダが存在すれば、空の状態で開いて **新規ファイル** と表示します。保存するとファイルが作成されます。

ペインの分割ボタン（右または下）を右クリックして **新規ファイル** を選ぶと、ペインのフォルダに空の `untitled.txt` を開きます。入力する前なら、ヘッダーの **ファイルを開く…** で、選んだファイルに置き換えられます。

ペイン上部のファイル名を右クリックすると、パス（フルパスまたはセッションのフォルダからの相対パス）のコピー、エディタや別のアプリで開く、ファイルマネージャーで表示、履歴の表示ができます。

ヘッダーには、ビューアと同じようにファイルのボタンがあります: 保存、長い行の折り返し、エディタで開く。未保存の編集があるときは点が付きます。ボタンが収まらない狭いペインでは、**ファイルの操作** メニューにまとまります。Markdown ファイルには **Markdown をプレビュー** もあり、隣にある画像も含めて表示どおりに見られます。もう一度クリックすると、テキストの編集に戻ります。**文字を小さく**と**文字を大きく**は、ターミナルとは別に、すべてのファイルとその Markdown プレビューの文字を拡大・縮小します。ファイルにフォーカスがあるときは、<kbd>⌘</kbd> <kbd>=</kbd>、<kbd>⌘</kbd> <kbd>-</kbd>、<kbd>⌘</kbd> <kbd>0</kbd>（Windows と Linux では <kbd>Ctrl</kbd>）でも同じことができます。

**ペイン › Markdownプレビューの切り替え** は、今いるペインで同じことをします。ペインは Kero を再起動してもプレビューのままです。エージェントは、プレビューで開いたり、プレビューを切り替えたりできます:

```sh
kero split --file plans/plan.md --preview
kero do pane.preview off
```

ペインはファイルを追いかけます: エージェントなど別のプログラムがファイルを書き換えると、ペインはその内容を表示します。ただし未保存の編集があればそれを残し、保存するときに上書きするかを確認します。

## ブラウザペイン

ブラウザペインは、ターミナルの横に Web ページを表示します: エージェントが作業中の開発サーバー、読んでいるドキュメント、開いたプルリクエストなど。ほかのペインと同じように分割、拡大、移動、クローズができ、Kero を再起動しても同じページに戻ります。

| アクション           | ショートカット                         |
| -------------------- | -------------------------------------- |
| 新規ブラウザペイン   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> |
| 場所を開く           | <kbd>⌘</kbd> <kbd>L</kbd>              |
| 要素を選ぶ           | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> |

ペインの分割ボタンを右クリックして **ブラウザ** を選ぶと、そのペインの隣に開きます。バーに `localhost:3000` や `github.com` のようなアドレス、または検索語を入力します。コマンドパレット（<kbd>⌘</kbd> <kbd>P</kbd>）に入力したアドレスも、新しいブラウザペインで開きます。<kbd>⌘</kbd> <kbd>F</kbd> でページ内のテキストを検索できます。

シェルやエージェントからは、`--url` で開きます:

```sh
kero split --url localhost:5173
kero new --url https://github.com/egoist/kero/pulls
kero split --url .backlog/pages/brief.html
```

このマシンのファイルのパス（`kero split` を実行したフォルダからの相対パス）や `file:///` URL は、ローカルページを表示します: 隣にある画像やスタイルシートはディスクから、CDN のスクリプトは Web から読み込まれます。ファイルがディスク上で変わると、ページを再読み込みします。ページと同じフォルダのスクリプトをモジュール（`<script type="module">`）として読み込んだり fetch したりすることは、ローカルページではできません: ブラウザはそれをサーバーからしか許しません。

その後、エージェントは `kero browser` で、あなたと同じようにページを使えます: クリック、入力、テキストの読み取り、読み込みの待機、スクリーンショット。[コマンドライン](./cli.md#browsing)を参照してください。

```sh
kero browser type 'input[name=q]' "kero" --enter
kero browser text main
```

<a id="sending-to-an-agent-from-a-page"></a>
### ページからエージェントに送る

ブラウザペインのローカルページ（`file:`）は、エージェントにメッセージを送れます。たとえば、エージェントが見せた判断資料（decision brief）で選んだ答えです。ページが `window.kero.send(message, options)` を呼ぶと、Kero はそのページを `kero split --url` で開いた（あるいは再表示した）エージェントのペインに、エージェントがプロンプトに戻った時点でメッセージを入力します。エージェントが [`kero browser wait-message`](./cli.md#browsing) で待っていれば、そのエージェントに渡します。エージェントが開いたのではないページでは、**セッションに送る…** と同じようにどのエージェントに送るかを尋ね、以後はそこに送ります。

メッセージには送り元が付きます: `Kero: message from database pick, from the page "Pick and send" (file:///…/send-to-agent.html) in browser pane 3f2a:` の後にテキストが続きます。`send` が返す Promise はエージェントがメッセージを受け取ると解決するので、ページは「送信済み」と表示できます。先にエージェントのペインが閉じると reject されます。

| オプション | 内容 |
| --------- | ---- |
| `label`   | メッセージが何か。ページのタイトルと URL の前に付きます |
| `mode`    | `"push"` は常にプロンプトに入力します。`"wait"` は `kero browser wait-message` のためだけに取っておきます。指定しなければ、待っているエージェントに渡し、いなければプロンプトに入力します |
| `onState` | Promise が解決する前に `"queued"`（エージェントが作業中: プロンプトに戻ったら届く）または `"waiting"`（`wait-message` のために保持）を受け取ります |

文字列でないメッセージは JSON として送られます。`window.kero` は Kero の中にだけあるので、ページはそれで送れるかどうかを判断し、送れないところではメッセージをクリップボードにコピーします:

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

`window` の `kero:delivery` イベントも、各メッセージの配信を `id`、`state`（`delivered` または `failed`）、`error` とともに伝えます。

### エージェントのために要素を選ぶ

ページのどこを変えてほしいかをエージェントに示すには、**要素を選ぶ** をオンにします: ペインのツールバーにある、枠とポインタのボタン、<kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd>（Windows と Linux では <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>C</kbd>）、またはペインメニューです。要素をクリックし、開いたメモにしてほしいことを書き、さらに選びます。書いている間にポインタを動かしても、メモはそのままです。**Parent** はその外側の要素を選びます: ボタンにポインタを置くと、選ばれる要素を点線で示します。**Send** か <kbd>Ctrl</kbd> <kbd>Enter</kbd> ですべて送り、<kbd>Esc</kbd> で止めます。

選んだ要素は、ページ自身のメッセージと同じ所に送られます: `kero split --url` でページを開いたエージェントに送られ、エージェントがプロンプトに戻ったときに入力されます。ページのメッセージを待っているエージェントがいれば、そのエージェントが受け取ります。エージェントが開いたのではないページでは、どのエージェントに送るかを一度だけ尋ね、それを覚えます。選ばずに閉じると、貼り付けられるようにコピーします。

CSS セレクタではエージェントがソースにたどり着けないことが多いので、各要素には、たどり着けるものを強い順に付けます:

1. **ソースの位置**（ページが分かる場合）: Svelte と React 18 の開発ビルドが示すファイルと行、React 19 のスタックからソースマップでたどった行、Vue ではコンポーネントのファイルとその中のタグの行、`data-v-inspector` などインスペクタプラグインが付ける属性。Kero は行をファイルと照合し、タグが別の行にあれば直します。ローカルファイルのページでは、ファイル内の開始タグの行です。どちらもないときは、ないことをメッセージに書くので、エージェントはすぐにヒントを使います。
2. **検索のヒント**: ソースにそのまま書かれていそうな、grep するための文字列: class 属性、id、テスト id、ラベル、name、リンク、プレースホルダ、要素自身のテキストの 1〜2 個のフレーズ（4〜8 語で、語の切れ目で終わり、’ や — のようにソースでエンティティとして書かれうる文字を含まないもの）。ページ内でその要素だけが持つものには印を付けます。
3. **ページ上の位置**: 上にあるランドマークと見出し、同じ種類の何番目か（「`<main>` 内、見出し "Pricing" の下、`div.card` 3 個中 2 番目」）、親と兄弟、短く切った HTML。そこにある `<script>`、`<style>`、`<svg>`、`<template>`、`<noscript>` はタグと属性を残し、中身は `/* 1.2 KB omitted */` のように大きさだけを示します。
4. セレクタと DOM パス、ページの URL とビューポート、主な計算済みスタイル、枠で囲んだ要素の画像。

画像は一時フォルダ内の `kero-picks` フォルダの PNG ファイルで（1 日後に削除します）、メッセージにそのパスが載るので、ターミナルのエージェントはパスで開けます。選択バーの **Screenshots** は 2 種類を撮ります: **Elements** は各要素の周りに余白を付けて枠で囲んだ画像（スクロールして見えなくなった要素は、見える所までスクロールして撮り、あとで戻します）、**Page** は見えているままのページで、各要素をメッセージと同じ番号付きの枠で囲みます。どちらもオンで、オフにしたものは次の選択でも覚えています。

コンポーネントの情報は開発ビルドだけが持ちます: 本番ビルドでは、ヒントとそれ以降を送ります。

<a id="blocking-ads"></a>

### 広告ブロック

ブラウザペインは、フィルタリストで広告とトラッカーをブロックします。ほかを選ばない限り、[EasyList](https://easylist.to) と EasyPrivacy を使います。Kero は最初にブラウザペインを開いたときにリストをダウンロードし、毎週更新します。リストの準備ができる前に開いたページは、準備ができ次第、次の読み込みから適用されます。

リストは **設定 › ブラウザ** の **広告をブロック** で選びます: Cookie の通知、ソーシャルボタン、迷惑な要素、中国語サイトの広告のリストもあります。ほかの URL のリストは、`browser-filter-list` として[構成](./configuration.md#browser)に追加します。リストを増やすほど多くを隠しますが、ときどきページが崩れることもあります。

Kero がブロックするのは、WebKit のコンテンツブロッカーで扱えるものです: 広告やトラッカーのサーバーへのリクエストと、リストが隠す要素。テキストで要素を隠すものや、ページが読み込む内容を書き換えるものなど、ブロッカー独自のスクリプトが必要なフィルタは除外されます。リストがブロックするドメインのページは、ブラウザペインで開きません。

ブロックせずに閲覧するには、**設定 › ブラウザ** で **広告をブロック** をオフにするか、[構成](./configuration.md#browser)で `browser-block-ads = false` を設定します。開いているページは、再読み込みするまで読み込んだ内容を保ちます。

<a id="signing-in-with-your-browsers-cookies"></a>
### ブラウザの Cookie でサインインする

ブラウザペインは独自の Cookie を持つため、Chrome や Edge でサインインしているサイトでも、ペインではもう一度サインインを求められます。サイトごとにサインインする代わりに、ブラウザの Cookie をペインに取り込めます:

1. Chrome（144 以降）または Edge で `chrome://inspect/#remote-debugging`（Edge では `edge://inspect/#remote-debugging`）を開き、リモートデバッグをオンにします。ブラウザを終了するまで有効です。
2. **設定 › ブラウザ** の **サインイン** で、ブラウザの横の **インポート** をクリックするか、`kero browser cookies import chrome`（または `edge`）を実行します。
3. ブラウザが接続を許可するか尋ねるので、許可します。

Kero はブラウザが既定とみなすプロファイルの Cookie をコピーし、ペインはブラウザでサインインしているサイトにサインインした状態になります。ブラウザには何も変更を加えず、Cookie を読んで切断します。

多くのウェブアプリは、サインインを Cookie ではなくサイトの localStorage や IndexedDB に保存します。ブラウザはサイトのストレージをそのサイトのページにしか渡さないため、Kero はブラウザで開いているタブのサイトのストレージもコピーします: インポートの前にそのアプリをブラウザで開き、インポート後にペインを再読み込みしてください。`kero browser cookies import chrome --domain app.example.com` は 1 つのサイトの分をコピーし、そのサイトのタブが開いていなければ、読み取るあいだだけ見えないタブを開きます。サイトのデータベースは、ペインにある同じ名前のものを置き換えます。8 MB を超えるデータベースや、サイトの暗号鍵のようにブラウザが自分で保持していてページからコピーできない値は除外し、何を除外したかを知らせます。サインインが切れたら、もう一度インポートしてください。ペインは Kero を再起動しても Cookie を保持し、`kero browser` のコマンドはこれまでどおりページで使えます。

Chrome と Edge は、ほかのプログラムがプロファイルを開いたり Cookie のファイルを読んだりすることを許さず、そのプロファイルでは `--remote-debugging-port` も無視します。`chrome://inspect` のスイッチが唯一の方法で、接続のたびに確認されます。Cookie はサインインそのものです: セッション内のエージェントも、あなたと同じようにページや `kero browser cookies save` を使えます。

<a id="driving-panes-with-devtools-tools"></a>
### DevTools のツールでペインを操作する

Windows では、ブラウザペインが DevTools ポート（Chrome が `--remote-debugging-port` で開くもの）を開けます。DevTools プロトコルを使うツール、[Playwright](https://playwright.dev) の `connectOverCDP`、[chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp)、Puppeteer、Lighthouse から使えます。**設定 › ブラウザ** で **DevTools ポート** をオンにするか、[構成](./configuration.md#browser)で `browser-devtools = true` を設定し、Kero を終了して起動し直してください: ペインのブラウザは起動時にポートを開きます。

各ペインは 1 つのブラウザのページなので、すべてのペインが 1 つのポートを共有します。`browser-devtools-port` で変えない限り `http://127.0.0.1:9222` です。ツールはペインを URL かタイトルで選びます。`kero browser devtools` は、ポートとそこでの各ペインのページを表示します:

```sh
$ kero browser devtools
http://127.0.0.1:9222
3f2a	8C1D…	http://localhost:5173/	Vite App
9b41	E07A…	https://github.com/egoist/kero	GitHub - egoist/kero
```

`--json` は `endpoint`、`port`、`browser`（ブラウザの WebSocket URL）と、ペインごとに `pane`、`session`、`url`、`title`、`target`、`webSocketDebuggerUrl` を表示します。まだページを表示していないペインにはターゲットがありません: 一度表示してください。

Playwright では、ポートに接続し、ペインのページを URL で選びます:

```js
import { chromium } from "playwright";

const browser = await chromium.connectOverCDP("http://127.0.0.1:9222");
const page = browser.contexts()[0].pages().find((p) => p.url().startsWith("http://localhost:5173"));
await page.getByRole("button", { name: "Add" }).click();
await browser.close(); // 切断するだけで、ペインは開いたまま
```

chrome-devtools-mcp には、ポートを `--browser-url` として渡します（Claude Code、またはほかのエージェントの MCP 設定で）:

```sh
claude mcp add chrome-devtools -- npx chrome-devtools-mcp@latest --browser-url=http://127.0.0.1:9222
```

```json
{ "mcpServers": { "chrome-devtools": { "command": "npx", "args": ["chrome-devtools-mcp@latest", "--browser-url=http://127.0.0.1:9222"] } } }
```

エージェントはペインのページを一覧し、URL かタイトルで 1 つを選びます。ペインは `kero split --url` などで Kero で開いてください: ツールが自分で開く新しいタブやウィンドウはペインではなく、WebView2 が拒むこともあります。

ポートは `127.0.0.1` だけで待ち受けますが、このコンピュータ上のどのプログラムも、ペインのページをサインインした状態のまま確認なしに使えるようになります: 使うあいだだけオンにしてください。ポートは Windows だけです: macOS と Linux のシステムの Web ビューはポートを開かないので、そこでは `kero browser` でペインを操作します。
