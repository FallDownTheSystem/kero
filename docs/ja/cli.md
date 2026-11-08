---
title: コマンドライン
description: kero CLI で、シェルやスクリプトから Kero のセッションとペインを操作します。
---

# コマンドライン

`kero` コマンドは、あなたのシェル、スクリプト、エージェントなど、どのシェルからでもセッションを操作できます。**Kero › コマンドラインツールをインストール…** でインストールします。

## 対象

ほとんどのコマンドは**対象**を取ります: セッションかペインです。対象には次のものを指定できます:

- ID、またはその一意な前方部分
- セッションの名前
- セッション（フォーカスのあるペインを表す）

Kero の中では、`$KERO_SESSION` と `$KERO_PANE` がコマンドを実行するセッションとペインを示し、それがデフォルトの対象になります。

## セッションとペイン

```sh
kero ls                          # すべてのセッションとそのペイン
kero new -n api -d ~/code/api    # セッションを開始し、その ID を出力
kero new -- npm run dev          # コマンドを実行するセッションを開始
kero split --down -- npm test    # このペインの横にペイン
kero split --url localhost:3000  # ページを表示するブラウザペイン
kero split --file src/main.go    # ファイルのエディタペイン
kero split --file plan.md --preview   # Markdown ファイルを表示どおりに
kero split --url docs/brief.html # ローカルの HTML ページ
kero new --file notes/plan.md    # 独立したエディタセッション。保存時にファイルを作成
kero rename api backend          # セッション名を変更
kero focus backend               # アプリで表示
kero close backend               # セッションを終える（ペインを指定するとそのペイン）
kero close fix-login --remove-worktree   # 専用のワークツリーも削除する
kero restart <pane>              # 終了したペインのシェルを再び開始
kero worktree list               # ワークツリー: セッション、Kero が作ったか、削除で失うもの、クリーンアップ
kero worktree status             # このセッションのワークツリー、セットアップ、プルリクエスト、ハンドオフ
kero worktree handoff fix-login  # このエージェントのセッションを専用のワークツリーに移す
kero new --orchestrator --goal "ログインページを出す"   # 子セッションを開始するオーケストレーター
kero orchestrator backend [on|off]  # セッションをオーケストレーターにする、または戻す
kero close boss --children       # オーケストレーターとその子を閉じる
kero report --to-parent "完了: テスト成功"   # 子の要約をオーケストレーターへ
```

`kero worktree handoff <branch>` は、起点のブランチ `--base <ref>` と、ワークツリーで再び起動したエージェントが続ける内容 `--prompt <text|->` を取ります（[詳しく](./changes.md#moving-a-session-into-a-worktree)）。

`kero new` のオプション:

| フラグ                      | 意味                                            |
| --------------------------- | ----------------------------------------------- |
| `-p`, `--project <name\|id>` | 入れるプロジェクト（デフォルト: ディレクトリから決まる） |
| `-n`, `--name <name>`       | セッションに名前を付ける                        |
| `-d`, `--dir <dir>`         | そこで開始する（デフォルト: 現在のディレクトリ） |
| `-f`, `--focus`             | アプリで表示する                                |
| `-w`, `--wait`              | コマンドの終了を待ち、その終了ステータスで終了する |
| `-u`, `--url <url>`         | シェルの代わりに、ページを表示するブラウザを開く。ローカルファイルのパスならそのファイルを表示 |
| `--file <path>`             | シェルの代わりに、ファイルのエディタを開く      |
| `--preview`                 | `--file` の Markdown ファイルを、テキストではなく表示どおりに見せる |
| `--worktree <branch>`       | 専用の git ワークツリーで、新しいブランチで開始する（[詳しく](./changes.md#sessions-in-their-own-worktree)） |
| `--base <ref>`              | ワークツリーのブランチの起点（既定: デフォルトブランチ） |
| `--orchestrator`            | [オーケストレーター](./agents.md#orchestrator-sessions)を開始する。エージェント（指定したコマンドか `default-agent`）は役割プロンプトから始める |
| `--goal <text>`             | オーケストレーターの目標                        |
| `--parent <session\|none>`  | 子として属するオーケストレーター（既定: 実行したセッションのもの）。`none` でどれにも属さない |

## 入力と読み取り

```sh
kero send api "npm run migrate" --enter   # テキストを入力してから Enter を押す
kero send api -e 'y\n'                    # \n \r \t \e \xHH のエスケープを解釈
kero key api ctrl+c                       # キーを押す
kero read api -n 40                       # 画面の最後の 40 行
kero read api -a                          # スクロールバックを含む画面
```

`kero key` は `enter`、`tab`、`escape`、`up`、`down`、`left`、`right`、`backspace` と、`ctrl+c` や `ctrl+d` のような修飾キーを理解します。

## 待機

```sh
kero wait api                             # 作業が止まるまで
kero wait api --for waiting,error --timeout 10m
kero prompt review "summarize the diff"   # 送信 + Enter、待機、画面を出力
```

`kero wait` は戻るときにペインの状態を出力します。`kero prompt` はテキストと Enter を送り、ペインの作業が止まるまで待ってから画面を出力します。ほかのエージェントに何かを尋ねる最も手早い方法です。

### コマンドの終了を待つ

`make test` のようなコマンドは*作業中*と表示されないので、終了を待つには `--wait` を付けて開始します。すると `kero` はそのコマンド自身の終了ステータスで終了します:

```sh
if kero split --down --wait -- make test; then
  echo "tests passed"
fi
```

`kero wait --exit` は、開始時のコマンドをすでに実行しているペインに対して同じことをします:

```sh
pane=$(kero split --down -- go test ./...)
# … ほかのことをする …
kero wait "$pane" --exit --timeout 10m
echo "go test exited with $?"
```

コマンドの終了後も出力とともにペインは開いたままなので、`kero read` で読めます。コマンドの終了を判断できない `cmd` をシェルにしている場合、コマンドの終了待ちは使えません。

## イベントの監視

`kero events` は、セッションとペインに起きたことを起きたときに 1 イベント 1 行で出力し、止めるまで続けます。スクリプトは `kero status` をポーリングせずに、すべてのエージェントを一度に追えます。

```sh
kero events                               # すべてのセッション
kero events api                           # 1 つのセッション、または 1 つのペイン
kero events -p kero                       # 1 つのプロジェクトのセッション
kero events --for waiting,error           # これらの状態になったペインだけ
kero events --json | jq -r .name          # 1 行に 1 つの JSON オブジェクト
kero events --for waiting --once          # 最初の 1 件で終了
```

```
12:04:31  api x78n9ss  working → waiting  Allow Bash: rm -rf dist?
12:04:40  api  renamed api → backend
```

| イベント | いつ | フィールド |
| --- | --- | --- |
| `pane.status` | ペインの状態が変わったとき | `pane`、`from`、`to`、`message`、`kind`、`agent`。exited になったときは `exitCode` も |
| `command.exited` | ペインの開始時のコマンドが終わり、シェルが続くとき | `pane`、`exitCode` |
| `pane.created`、`pane.closed` | ペインが開始または終了したとき | `pane` |
| `session.created`、`session.closed` | セッションが開始または終了したとき | |
| `session.renamed` | セッションの名前が変わったとき | `from`、`to` |
| `session.moved` | セッションが別のプロジェクトに移ったとき | `from`、`to`（プロジェクトの ID） |
| `notice` | セッション内のプログラムがデスクトップ通知を送ったとき | `pane`、`title`、`body` |

`--json` では、どのイベントにも `event`、`time`（unix ミリ秒）、`session`、`project`、`name`（サイドバーに表示されるセッション名）が付きます。

`kero events` が報告するのは今からの変化です。すでにあるものは `kero ls --json` で確認します。対象を指定すると、対象が閉じたときに止まります。`--once` は最初に一致したイベントで終了し、対象が先に閉じるか `--timeout` が過ぎると `1` で終了します。kerod は状態が落ち着いてから送るので、一瞬だけの状態（短い作業中など）は出ないことがあります。

<a id="browsing"></a>

## ブラウザの操作

`kero browser` は、`kero send` と `kero read` がターミナルを操作するのと同じように、ブラウザペインのページを操作します: クリック、入力、ページの読み取り、スクリーンショット。対象はブラウザペイン、またはセッション（そのブラウザ）です。対象を指定しなければ、実行したセッションのブラウザを操作します。

```sh
kero split --url localhost:3000            # ページを開き、ペインの ID を出力
kero browser open localhost:3000/todos     # ページへ移動し、読み込みを待つ
kero browser type '#new-todo' "Buy milk" --enter
kero browser wait '.todo-list li'          # 要素が表示されるまで
kero browser wait --text "1 item left"     # テキストが表示されるまで
kero browser text                          # ページのテキスト
kero browser text h1                       # 要素のテキスト
kero browser click 'button[type=submit]'
kero browser screenshot -o page.png
kero browser eval 'document.title'
kero browser console --errors              # ページが投げたエラーとログ、ファイルと行つき
kero browser network --failed              # 失敗したか 4xx/5xx を返したリクエスト
kero browser emulate device iphone-15      # スマートフォンと同じようにページを表示
```

| コマンド                        | 動作 |
| ------------------------------- | ---- |
| `open <url>`                    | ページへ移動し、読み込みを待つ |
| `back`、`forward`、`reload`     | 戻る、進む、または再読み込みして、それを待つ |
| `text [selector]`               | ページの表示テキスト、または最初に一致した要素のテキストを出力 |
| `html [selector]`               | ページ、または要素の HTML を出力 |
| `click <selector>`              | マウスと同じように要素をクリック |
| `type <selector> <text…>`       | フィールドの内容を置き換えてテキストを入力。`--enter` を付けるとその後 Enter を押し、フォームを送信する |
| `wait [selector] [--text text]` | 要素かテキストが表示されるまで待つ。どちらもなければページの読み込みまで |
| `eval <js…>`                    | ページで JavaScript を実行し、その値を出力: 文字列はそのまま、それ以外は JSON |
| `screenshot [-o file.png]`      | ページの画像を保存し、その場所を出力。`-o -` で標準出力に書き出す |
| `cookies import <chrome\|edge>`  | 実行中の自分のブラウザの Cookie と、開いているタブのサイトの localStorage と IndexedDB をコピーし、ペインをブラウザと同じサイトにサインインさせる。`--domain` でサイトの分だけ（[手順](./panes.md#signing-in-with-your-browsers-cookies)） |
| `wait-message [--json]`         | ページが [`window.kero.send`](./panes.md#sending-to-an-agent-from-a-page) で送る次のメッセージを待ち、ページの情報を付けて表示します。`--json` は `id`、`from`、`label`、`url`、`title`、`text`、`at` を表示します |
| `pick`                          | 要素を選ぶモードをオンにし、ユーザーが要素を選んでメモを書けるようにする。選んだ要素はページのエージェントへのメッセージとして届く（[詳しく](./panes.md#エージェントのために要素を選ぶ)） |
| `console [--errors] [--clear]`  | ページが `console.*` で出力したもの、投げたエラー、誰も捕まえなかった Promise、CORS エラーや読み込みの失敗などブラウザが伝えたものを、それぞれファイルと行つきで出力。`--errors` はエラーだけ、`--clear` は出力後にログを空にする |
| `network [--failed] [--url text] [--clear]` | ページのリクエストを 1 行ずつ出力: `#id`、メソッド、ステータス、種類、サイズ、時間、URL と、失敗した理由（`net::ERR_…`、`CORS …`、`blocked: …`）。`--failed` は失敗したか 4xx・5xx を返したものだけ、`--url` は URL にそのテキストを含むものだけ |
| `network body <id> [-o file]`   | 一覧にあるレスポンスの本文を出力、またはファイルに保存 |
| `emulate device <name>`         | スマートフォンやタブレットと同じようにページを表示: ビューポート、スケール、タッチ、ユーザーエージェント。`emulate device` で一覧（`iphone-se`、`iphone-15`、`iphone-15-pro-max`、`pixel-8`、`galaxy-s24`、`ipad`、`ipad-pro`） |
| `emulate viewport <w>x<h>`      | そのサイズでページをレイアウト。`--scale` は CSS ピクセルあたりのデバイスのピクセル、`--mobile` はスマートフォンのようにレイアウト、`--touch` はタッチを受け付ける。`none` で元に戻す |
| `emulate user-agent <ua…>`      | 別のユーザーエージェントを送る。`none` で元に戻す |
| `emulate network <preset>`      | ネットワークを遅くする（`slow-3g`、`3g`、`4g`）、`offline` にする、または戻す（`none`） |
| `emulate [reset]`               | ページの表示のされ方を出力。`reset` ですべて元に戻す |

`wait-message` は `--timeout` を指定しない限り届くまで待ち、何も届かなければ `timed out` で失敗します: エージェントは、シェルツール自体のタイムアウトより短いタイムアウトで待ち直します。Remote Control は不要です。

セレクタは CSS セレクタです。シェルでは引用符で囲んでください。要素が必要なコマンドは要素が現れるのを待ち、`--timeout`（デフォルト `10s`）でそれらとページの読み込みにかけてよい時間を設定します。`type` と `eval` は、`-` で標準入力からテキストを受け取ります。

クリックと入力は本物のマウスとキーボードと同じイベントを送るので、React などのフレームワークで作られたページもそれを受け取ります。スクリーンショットにはペインが画面に表示されている必要があります: 先に `kero focus` で表示してください。それ以外は、表示されているかどうかにかかわらず、どのブラウザペインでも動きます。

`console` と `network` は、ペインのページが作られてから（最初の読み込みも含め）のものを、それぞれ最新の 1,000 件まで保ちます。`--clear` で空にすると、次に読むときには再読み込みやクリックで起きたことだけが出ます。`--json` はそれぞれの項目を出力し、`emulate` の状態も出力します。`emulate` で設定したものは、ペインが開いている間、再読み込みやほかのページへの移動のあとも続きます。`network` と `emulate` は DevTools Protocol を使い、ブラウザペインでこれがあるのは Windows だけです。macOS と Linux では、`console` には現在のドキュメントが出力したものが入ります。

`kero do` と同じく、`kero browser` には **設定 › エージェント** のリモート操作と、アプリが開いていることが必要です。

Windows では、Playwright や chrome-devtools-mcp など DevTools プロトコルを使うツールも、**DevTools ポート** が開くポートからペインを操作できます（[手順](./panes.md#driving-panes-with-devtools-tools)）。`kero browser devtools` はポートと、そこでの各ペインのページをターゲット ID、URL、タイトルで表示します。`--json` で JSON として表示します。リモート操作は不要です。

## 過去の会話

```sh
kero history                     # ペインで動いたエージェントの会話を、最後に動いたものから順に
kero history -p kero -n 50       # ひとつのプロジェクトの会話を、最大 50 件
kero history login --json        # 「login」に一致するものを JSON で
kero resume 13b9                 # 再開する: ID、その先頭部分、または名前で
kero resume fix-login -f         # 再開して、アプリに表示する
```

`kero resume` は、その会話を再開するセッションを会話のディレクトリとプロジェクトで開始し、セッションの ID を表示します。まだペインで動いている会話なら、そのセッションを返します。[過去の会話](./sessions.md#past-conversations)を参照してください。

## 状態

```sh
kero status api --json                    # 状態、プログラム、ディレクトリ
kero report working "Running migrations"  # 自分の状態を報告
kero notify "Need your input"             # このペインを対応が必要なものとして示す
```

状態については[状態と通知](./status.md)を参照してください。

## 変更

```sh
kero diff                                 # ペインの未コミットの変更を git diff の形式で
kero diff --turn                          # エージェントが直前のターンで変更したもの
kero diff review --branch                 # デフォルトブランチ以降
```

[変更](./changes.md)を参照してください。

## 下書きのレビュー

プルリクエストをレビューするエージェントは、コメントを[下書きのレビュー](./changes.md#draft-reviews)に書き、それを Kero から投稿します。行はプルリクエストのヘッドでのファイルの行 (`--base` ではベースの行) で、プルリクエストの差分の中にある必要があります:

```sh
kero pr comment src/parse.go:42 "This drops the last token."
kero pr comment src/parse.go:40-44 -      # 本文は標準入力から
kero pr comment src/api.go "Split this file?"   # ファイル全体
kero pr summary "Two bugs in the parser."
kero pr show                              # 下書きと、各コメントの id
kero pr edit 3 "Clearer text"
kero pr delete 3
```

Kero から始めたレビューでは、ペインがそのプルリクエストを知っています。それ以外では、レビューに下書きがあれば `--pr <番号>` で指定します。下書きはプルリクエストの **下書きを開始** で始められます。

## レビューガイド

エージェントは `kero pr guide` で、プルリクエストの[レビューガイド](./changes.md#review-guides) (ガイドタブの案内) を書きます。ストップは追加した順に並び、`--at <n>` で n 番目に置けます。行はプルリクエストの差分を含む必要があり、パスだけならファイル全体です。GitHub には何も送られません:

```sh
kero pr guide summary "Moves retries onto a queue."
kero pr guide stop src/queue.go:10-42 --title "The new queue" "Why it exists."
kero pr guide stop src/api.go:88 --title "Who enqueues" --closely   # Look closely
kero pr guide stop testdata/golden.txt --title "Snapshots"          # the whole file
kero pr guide show                        # the guide, each stop with its id
kero pr guide edit 2 --title "Clearer"    # or move 2 --at 1, delete 2
kero pr guide clear                       # start the tour again
```

Kero から始めたガイドでは、ペインがそのプルリクエストを知っています。それ以外では `--pr <番号>` で指定します。

Kero がセッションのエージェントのために見守る、セッションの[プルリクエスト](./changes.md#セッションのプルリクエスト):

```sh
kero pr link                 # セッションのブランチの開いたプルリクエスト
kero pr link 123             # または番号か URL
kero pr status --json        # 状況: チェック、失敗したチェック、コンフリクト
kero pr unlink
```

`gh` にサインインしたユーザーとして、GitHub のプルリクエストを操作します。番号を指定するか、省略してセッションにリンクされたものを対象にします。マージは、指定がなければリポジトリの既定の方法（**リポジトリの設定**のもの、なければ squash、merge、rebase のうちリポジトリが許す最初のもの）で行います:

```sh
kero pr state 123                          # オープンか、ドラフトか、マージできるか、どの方法で
kero pr ready 123                          # または draft、close、reopen
kero pr update-branch 123 --rebase         # ブランチをベースに追いつかせる
kero pr merge 123 --squash --delete-branch # または --merge、--rebase。--head <sha> はそのコミットでだけマージ
kero pr merge 123 --auto                   # 要件を満たしたらマージ。--disable-auto で止める
kero pr delete-branch 123                  # クローズかマージのあとで
kero pr checkout 123                       # そのブランチをここに。変更のあるワークツリーでは断る
kero pr checkout 123 --worktree -- claude  # 専用のワークツリーで新しいセッション。その ID を出力
```

そのプルリクエストを持つワークツリーがすでにあれば、それをそのまま使います: `kero pr checkout` はその場所を伝え、`--worktree` ならそこで動いているセッションを出力し、なければそこでセッションを始めます。

## 構成とセットアップ

```sh
kero config path                 # 設定ファイルの場所
kero config show                 # すべてのキーとその値
kero config show --defaults      # すべてのキーの初期値
kero config edit                 # $EDITOR で開く
kero config check                # 不明なキー、不正な値、agents.toml の問題を報告
kero projects                    # プロジェクトをサイドバーの順に一覧
kero projects move <project> top # 移動: top、bottom、up、down、または位置 (1 が先頭)
kero skill setup                 # コーディングエージェントに kero の使い方を教える
kero daemon status|stop|restart  # kerod を管理
kero daemon install-service      # マシンの起動とともに kerod を実行（systemd、launchd）
kero daemon serve                # サービスマネージャ向けに kerod をフォアグラウンドで実行
kero clients                     # kerod に接続しているもの
```

## ほかのマシン

コマンドの前に `-H <host>` を付けるか `$KERO_HOST` を設定すると、セッションに対するコマンドを別のマシンの kerod に向けられます: 追加したホスト、または `me@devbox` や `wsl://Ubuntu` のようなアドレスです。[リモートホスト](./remote.md)を参照してください。

```sh
kero hosts                       # 追加したホスト
kero hosts add devbox me@devbox  # 追加する（アプリにも表示される）
kero hosts check devbox          # 接続して、結果を表示
kero hosts install devbox        # SSH でそこに Kero をインストール
kero ls --all                    # このマシンとすべてのホストのセッション
kero -H devbox ls
kero -H devbox new -n review -- claude
kero -H devbox daemon update     # そこの Kero をこちらのバージョンにアップデート
```

ペインの中では、`kero` はアプリがどこにあっても、ペインが動いているマシンの kerod と通信します。

`kero ls --all` はすべてのホストに同時に問い合わせ、それぞれのセッションを `-H` に渡す名前（このマシンは `local`）の下に一覧表示します。届かないホストは、セッションの代わりにその理由を表示します。`--json` を付けると `{host, addr, error, sessions}` のリストを出力します。

<a id="managing-kero"></a>
## Kero を管理する

サイドバーとそのメニュー、設定でできることを、シェルから次のコマンドで行えます。[Ama](./agents.md#ヘルパーエージェント-ama) もこれらを使います:

```sh
kero ls -p api --status waiting                # プロジェクトのセッションを状態で絞り込む
kero projects add ~/code/api -n api            # id を表示
kero projects rename api backend
kero projects remove backend --force           # セッションも閉じる
kero move <session> <project>
kero close --all -p api --status idle,exited --dry-run   # 閉じる対象を確認
kero close --all -p api --status idle,exited
kero worktree create fix-login -p api -- claude "fix the login"
kero worktree remove fix-login -p api --branch # ブランチも削除
kero worktree cleanup --dry-run                # 自動クリーンアップで消えるもの
kero worktree prune -p api
kero pr list -p api
kero pr review 123 -p api                      # レビュータブと同じように開始
kero config get default-agent
kero config set notify-on-done false           # 設定と同じように書き込む
```

`--all`、`-p`、`--status` を付けた `kero close` は、Ama のセッションと、コマンドを実行しているセッションは閉じません。`kero worktree remove` は履歴タブと同じようにワークツリーを削除します。Kero がセッション用に作ったワークツリーはそのセッションを閉じ、他のセッションが使っているものは残ります。`kero pr review` はアプリが開いている必要があり、Ama 以外からはリモート操作も必要です。値なしの `kero config set` はデフォルトに戻します。リスト設定は値ごとに引数を渡し、何も渡さなければ空になります。

## アプリのアクションを実行する

`kero do` は、メニューと同じアプリのアクションをどれでも実行します。**設定 › エージェント** のリモート操作（デフォルトでオン）が必要です。

```sh
kero actions                     # 一覧を表示
kero do pane.split.right
kero do pane.browser url=localhost:3000
kero do sidebar.toggle
kero do files.show path=src/main.go
```

相対パスの `path=` または `dir=` は、`kero do` を実行したディレクトリを基準にします。

`kero do pane.preview` は、エディタペインの Markdown ファイルを表示どおりに見せるか、テキストに戻します: `on`、`off`、どちらも指定しなければ切り替えます。`pane=` で指定したペイン、なければセッションの Markdown ファイルのエディタペインが対象です。`kero split --preview` と同じく kerod が保持するペインの状態を設定するので、リモートコントロールもアプリの起動も不要です。

```sh
kero do pane.preview on
kero do pane.preview off pane=4f2a
```

## 終了ステータス

| コード | 意味                                 |
| ------ | ------------------------------------ |
| `0`    | 成功                                 |
| `1`    | 失敗                                 |
| `2`    | 使い方の誤り                         |
| `3`    | 対象が存在しない、またはあいまい     |
| `4`    | kerod に接続できない                 |

プログラムを待つコマンド（`kero wait --exit` と `--wait`）は、代わりにそのプログラムの終了ステータスで終了します。ペインのシェルがエラーで終了したときの `kero wait` も同じです。タイムアウトした待機は `1` で終了します。

`ls`、`status`、`projects` に `--json` を付けると、機械で読める形式で出力します。
