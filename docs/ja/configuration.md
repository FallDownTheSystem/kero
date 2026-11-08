---
title: 設定
description: Kero の設定ファイルと、そのキーとデフォルト値。
---

# 設定

Kero は設定をひとつのプレーンテキストファイルに保存します。設定ウインドウ（<kbd>⌘</kbd> <kbd>,</kbd>）はコメントと順序を保ったままこのファイルを編集し、手で編集したときは Kero が読み込み直します。

## ファイル

Kero は次の順に優先して読み込みます:

1. `$KERO_CONFIG`
2. `$XDG_CONFIG_HOME/kero/config`
3. `~/.config/kero/config`

Windows でもファイルは `~/.config/kero/config`、つまり `C:\Users\<you>\.config\kero\config` です。`kero config path` でその場所を表示します。隣には `agents.toml`（[独自のエージェント](./status.md#your-own-agents)）と `ama`（[Ama のフォルダ](./ama.md#its-folder)）があります。ログやセッションなど Kero のほかのファイルは、データフォルダにあります（[トラブルシューティング](./troubleshooting.md#where-kero-keeps-things)）。

構文は Ghostty と同じです: 1 行に 1 つの `key = value`、コメントは `#`、リストを持つキーは行を繰り返します。Ghostty と同じ意味のキーには、Ghostty と同じ名前が付いています。

```ini
# ~/.config/kero/config
font-size = 14
theme = light:GitHub Light Default,dark:Catppuccin Mocha
cursor-style = bar

env = EDITOR=nvim
env = HOMEBREW_NO_AUTO_UPDATE=1

keybind = cmd+shift+d=pane.split.down
```

ファイルには変更したものだけを書けば十分です。それ以外はデフォルトのままです。

```sh
kero config edit              # $VISUAL、なければ $EDITOR、なければシステムのテキストエディタで開く
kero config show              # すべてのキーと現在の値、それぞれの働き
kero config show --defaults   # Kero の初期状態のすべてのキー
kero config get font-size     # 1 つのキーの値
kero config set font-size 14  # 設定と同じように 1 つ設定する。値なしならデフォルトに戻す
kero config check             # 不明なキー、不正な値、agents.toml の問題を報告
```

**設定 › 詳細 › すべての設定をリセット** は、キーボードショートカットも含めてすべてのキーをデフォルトに戻します。セッションはそのまま残ります。設定ウインドウの検索欄で、設定を名前やキーで探せます。

## 一般

| キー                    | デフォルト | 説明                                                          |
| ----------------------- | ---------- | ------------------------------------------------------------- |
| `new-session-directory` | `current`  | 新規セッションの開始場所: `current`、`project`、`home`        |
| `session-name`          | `focused`  | 名前を付けていないセッションの名前の付け方: `focused`、`lead`、`folder`（[詳しく](./sessions.md#how-an-unnamed-session-is-named)） |
| `sidebar-rows`          | `detailed` | サイドバーでのセッションの表示: `detailed` はステータスメッセージ、なければブランチと変更を 2 行目に表示、`compact` は 1 行で表示 |
| `sidebar-session-count` | `true`    | サイドバーのプロジェクト名の横に、含まれるセッションの数を表示 |
| `focus-follows-mouse`   | `false`    | ポインタが入ったペインにフォーカスを移す                      |
| `confirm-close`         | `true`     | プログラムが実行中のペインを閉じる前に確認する                |
| `restore-sessions`      | `true`     | 再起動のあとなど kerod が再起動したら、セッションを再び開始する |
| `resume-agents`         | `true`     | セッションの再開時に、各ペインで動いていたエージェントの会話を再開する |
| `session-history`       | `false`    | 終了したエージェントの会話をサイドバーに一覧し、再開やアーカイブができるようにする（[詳細](./sessions.md#ended-sessions)） |
| `macos-show-app-icon`   | `dock-and-menubar` | macOS で Kero のアイコンを表示する場所: `dock-and-menubar`、`dock`、`menubar`。メニューバーに表示している間は、ウインドウを閉じても Kero はそこで動き続ける。Windows と Linux では常に通知領域に表示し、ウインドウを閉じても同じく動き続ける |
| `keep-computer-awake`   | `off`              | このコンピュータをスリープさせない: `off`、`on`(Kero の実行中ずっと)、`agents`(このマシンでエージェントが作業している間)。コマンドパレットや、メニューバーまたは通知領域の Kero アイコンのメニューからも選べる。Mac の蓋を閉じるとスリープする |

## 外観

| キー                 | デフォルト | 説明                                                            |
| -------------------- | ---------- | --------------------------------------------------------------- |
| `window-theme`       | `system`   | アプリの外観: `system`、`light`、`dark`                         |
| `language`           | `system`   | アプリの言語: `system`、`en`、`ja`、`zh-Hans`                   |
| `window-translucent` | `true`     | ウインドウからデスクトップを透けて見せる                        |
| `app-font`           | （空）     | アプリ自身のテキストのフォント（ターミナルのものではない）: 空ならシステムのフォント。Inter、Geist、Mona Sans、Public Sans、IBM Plex Sans、Plus Jakarta Sans、Manrope、Figtree、DM Serif Display、Playfair Display が付属し、インストール済みのフォントならどれでも使えます |
| `sessions-sidebar`   | `left`     | セッションのサイドバーを置くウインドウの側: `left` または `right`。右サイドバーとビューアは反対側に置きます |
| `pane-folder`        | `fit`      | ペインのヘッダーにフォルダをどう示すか: `hidden`、`folder`（名前）、`full`（パス）、または幅に収まる限り多くを示す `fit` |
| `explorer-folder`    | `fit`      | ビューアのヘッダーにフォルダをどう示すか。値は `pane-folder` と同じ |
| `dim-unfocused-terminals` | `true` | セッションに複数のペインがあるとき、使用中でないターミナルを薄くする |
| `dim-unfocused-files` | `false` | セッションに複数のペインがあるとき、使用中でないエディタペインを薄くする |
| `dim-unfocused-web` | `false` | セッションに複数のペインがあるとき、使用中でないブラウザペインを薄くする |
| `dim-unfocused-explorer` | `false` | ビューアを使用していない間、薄くする |

## ターミナル

| キー                  | デフォルト | 説明                                                                     |
| --------------------- | ---------- | ------------------------------------------------------------------------ |
| `theme`               | `paper`    | ウインドウ、ペイン、ターミナルのテーマ: Kero のテーマ（`petal`、`mist`、`meadow`、`dusk`、`tide`、`apricot`、`aurora`、`dune`、`paper`、`plain`）、任意の Ghostty テーマ、または `light:paper,dark:dusk` のように外観ごとに |
| `file-theme`          | `terminal` | エディタのペインとビューアのコードの色: ターミナルのテーマには `terminal`、それ以外は `theme` と同じ書き方 |
| `font-family`         |            | 空欄なら Kero に付属の JetBrains Mono Nerd Font、`monospace` ならシステムのもの、またはインストール済みの任意のファミリー |
| `font-size`           | `13`       | ポイント単位                                                             |
| `font-weight`         | `400`      | `100` から `900`                                                         |
| `line-height`         | `1.22`     | フォントの行の高さに対する倍率                                           |
| `cell-width`          | `1`        | セルの幅。フォントの幅に対する倍率で、`0.8` から `2`。文字の大きさは変わらない |
| `font-ligatures`      | `true`     | フォントが描くとおりに文字をつなげる                                     |
| `font-feature`        |            | フォントの OpenType 機能。Ghostty と同じく、`ss01` で有効にし、`-calt` で無効にし、`cv05=2` で異体字を選ぶ。カンマ区切りか繰り返しで指定し、`font-ligatures` の後に適用される |
| `font-thicken`        | `false`    | Ghostty と同じく、線を太くする                                           |
| `minimum-contrast`    | `3`        | テキストが背景に対して保つ最小のコントラスト。`1`（なし）から `21`。Ghostty と同じく、暗い背景向けの色が明るい背景でも読めるようにする |
| `window-padding-color` | `extend` | ターミナルの文字の周りの余白に表示するもの。`background` はターミナルの背景、`extend` は隣のセルの色（背景のままの行を除く）、`extend-always` は常に隣のセルの色。Ghostty と同じ。`background` にすると、プログラムの下部のバーは 1 行の高さのままになる |
| `cursor-style`        | `block`    | `block`、`bar`、`underline`。プログラムが変更することがある              |
| `cursor-style-blink`  | `true`     | カーソルを点滅させる                                                     |
| `macos-option-as-alt` | `false`    | Option でプログラムに Alt（メタ）を送る                                  |
| `copy-on-select`      | `false`    | 選択したものをクリップボードにコピーする                                 |
| `clipboard-read`      | `ask`      | プログラムによるクリップボードの読み取り（OSC 52）: `ask`、`allow`、`deny` |
| `right-click-action`  | `context-menu` | `context-menu`、`paste`、`copy-or-paste`                             |
| `mouse-scroll-multiplier` | `precision:1,discrete:3` | スクロールの量。Ghostty と同じく、`precision:` はトラックパッドのスクロールの倍率、`discrete:` はマウスホイール 1 ノッチでスクロールする行数（Windows では 3 行設定のときの 1 ノッチ）。数値を 1 つだけ書くと両方に適用。`0.01` から `10000` |

`cell-width = 1.03` で文字の間隔が少し広がり、`0.95` で少し狭まります。`1` 未満では
幅の広い文字どうしが接することがあります。セルの幅はピクセル単位に丸められるため、
近い値では見た目が同じになったり、1 ピクセル違ったりします。

独自のテーマは、Ghostty のテーマフォルダにある Ghostty 形式のファイルです: `$XDG_CONFIG_HOME/ghostty/themes`（設定されている場合）、それ以外は Windows で `%LOCALAPPDATA%\ghostty\themes`、その他で `~/.config/ghostty/themes`。設定 › 外観で、Ghostty のテーマのメニューでテーマにポインタを置くとウインドウでプレビューでき、「テーマをカスタマイズ」で使用中のテーマのすべての色を編集して、独自のテーマとしてそこに保存できます。Kero 独自のテーマの色は、選ばない限りテーマのほかの色に従います:

- **背景の色**: カードの下で上から下へ混ざる 2〜4 色（Aurora のように）。選ばなければテーマの背景の 2 つの濃淡。
- **背景の光**: ウインドウを右上の角から照らす色。選ばなければハイライトの背景。
- **ハイライト**: アイコン、輪郭、リンク、フォーカスリング、選択した行の淡い色。選ばなければテーマのカーソルの色（カーソルが灰色ならテーマの青）。
- **ハイライトの背景**: 主要なボタン、ハイライトされたメニュー項目、ピルの面。文字は白かほぼ黒の読みやすい方。選ばなければハイライト。カーソルを明るくしたまま、ボタンを落ち着いた色にできます。

Kero はこれらを `# kero-wash:`、`# kero-glow:`、`# kero-highlight:`、`# kero-highlight-bg:` の行に保存し、Ghostty はその行をコメントとして読みます。`# kero-accent:` の行で保存したテーマは、それを両方のハイライトとして使います。「書き出す」は編集中のテーマを、これらの行も含めファイルのテキストとしてコピーします。「読み込む…」はそのようなテキストを貼り付けて編集中のテーマに取り込み、「保存」をクリックしたときだけ保存されます。

ファイルのテーマはエディタのペインとエクスプローラを彩ります。そこに表示されるコードはファイルのテーマの 16 色を使います: キーワードはマゼンタ、文字列は緑、型は黄、関数は青、コメントは明るい黒。ファイルのテーマは設定 › 外観 › ファイルの色で選びます。既定ではターミナルのテーマと同じです。

## シェル

| キー               | デフォルト | 説明                                                           |
| ------------------ | ---------- | -------------------------------------------------------------- |
| `command`          |            | 新しいペインのシェル。空欄ならログインシェル                   |
| `env`              |            | `NAME=value` として、すべてのペインの環境変数に追加する。繰り返して指定する |
| `scrollback-limit` | `4`        | 各ペインが保持する出力（メガバイト単位）。`0` で保持しない     |

## 通知

| キー                | デフォルト | 説明                                           |
| ------------------- | ---------- | ---------------------------------------------- |
| `notify-on-waiting` | `true`     | セッションが対応を求めたとき                   |
| `notify-on-done`    | `true`     | エージェントが完了したとき                     |
| `notify-on-error`   | `true`     | プログラムが失敗を報告したとき                 |
| `notify-sound`      | `false`    | 通知とともにサウンドを再生する                 |
| `dock-badge`        | `true`     | 対応が必要なセッションの数を Dock アイコンに表示する |
| `dock-bounce`       | `false`    | セッションが対応を求めたときに Dock アイコンを 1 回弾ませる |
| `notify-children`   | `false`    | [オーケストレーター](./orchestrators.md)の子セッションも通知する。オフでは、オーケストレーター自身の入力待ちとエラーだけが通知される |

## エージェント

| キー             | デフォルト | 説明                                                |
| ---------------- | ---------- | --------------------------------------------------- |
| `remote-control` | `true`     | エージェントによるアプリの操作を許可する: `kero do`、`kero focus`、`kero browser`、`kero pr review`（[詳しく](./agents.md#remote-control)） |
| `agent`          | `claude`   | [新規セッションのダイアログ](./sessions.md#new-session-dialog)のエージェントセッションで起動できるコマンド。自分のコマンドや関数と同じようにシェルで実行されます（`cc` や `codex --full-auto` など）。エージェントごとに繰り返して指定する |
| `default-agent`  | `claude`   | 新規セッションのダイアログを開いたときに選ばれているエージェント |
| `orchestrator-prompt` |  | オーケストレーターの役割プロンプト（エージェントへの最初のメッセージ）。行の区切りは `\n`。`{goal}` は目標、`{agent}` は `default-agent`。空なら Kero のもの |
| `orchestrator-max-children` | `0` | 1 つのオーケストレーターの子が同時にいくつ動けるか。超えると `kero new` は拒否する。`0` で無制限 |
| `orchestrator-agent-args` |  | どのエージェントでも、オーケストレーターのエージェントのコマンドの後ろに付ける引数（`--model opus` など）。シェルが読むとおりに渡し、再開時にも付ける |
| `ama-agent` |  | [Ama](./ama.md) が次のチャットから実行するエージェントのコマンド。空なら `default-agent` のもの |
| `ama-agent-args` |  | どのエージェントでも、Ama のエージェントのコマンドの後ろに付ける引数（`--model sonnet` など）。再開時にも付ける |
| `ama-prompt` |  | Ama の役割プロンプト。Kero がそのフォルダの `CLAUDE.md` と `AGENTS.md` に書く。行の区切りは `\n`。空なら Kero のもの |
| `ama-font-size` | `0` | Ama のウインドウの文字サイズ（ポイント）。`0` で `font-size` より 1 ポイント小さく |
| `ama-hide-status-line` | `true` | Ama だけ Claude Code のステータスラインを隠す（Ama のフォルダの `.claude/settings.json` に空のものを書く）。あなた自身の Claude Code の設定は変わらない |
| `resume-command-claude` |  | Claude Code の会話を再開するコマンド。再開、[検索](./sessions.md#search)、セッションの復元で使われ、自分のコマンドや関数と同じようにシェルで実行されます（`cc --yolo --resume {session}` など）。中の `{session}` は会話の ID になり、なければ ID は末尾に付きます。空なら `claude --resume {session}` |
| `resume-command-codex` |  | Codex について同じ。空なら `codex resume {session}` |
| `resume-command-pi` |  | Pi について同じ。空なら `pi --session {session}` |
| `resume-command-opencode` |  | OpenCode について同じ。空なら `opencode --session {session}` |

## ホスト

アプリにセッションを表示し、`kero -H` で接続する、ほかのマシン。[リモートホスト](./remote.md)を参照してください。

| キー           | デフォルト | 説明 |
| -------------- | ---------- | ---- |
| `host`         |            | `name=address` 形式のホスト: SSH なら `me@devbox` や `ssh://…`、WSL なら `wsl://distro`、kerod の tailnet リスナーなら `http(s)://…`。後ろに ` remote-control` を付けるとそのプログラムにアプリの操作を許可し、` color=teal`（red、orange、amber、green、teal、blue、purple、pink）を付けるとそのセッションに色を付けられる。ホストごとに繰り返して指定する |
| `listen`       |            | ほかのマシンの Kero からこのマシンへの接続を許可する: Tailscale アドレスで待ち受けるなら `tailnet://:7373`、Tailscale Serve なら `tailnet-serve://`。それぞれ繰り返して指定する |
| `listen-allow` |            | マシンの所有者以外に接続を許可する相手: tailnet のログイン、`tag:name`、または tailnet 上の誰でもなら `*`。それぞれ繰り返して指定する |
| `remote-updates` | `false`  | ほかのマシンの Kero が、ここの Kero アプリをそのバージョンにアップデートすることを許可する（アプリは再起動する） |

## Git

| キー          | デフォルト | 説明                                                                                              |
| ------------- | ---------- | ------------------------------------------------------------------------------------------------- |
| `git-status`  | `true`     | 各セッションのブランチと[変更](./changes.md)を表示する                                            |
| `diff-layout` | `auto`     | 変更タブの差分: `unified`、`split`（左右に並べる）、または広くなると左右に並べる `auto`            |
| `wrap-lines`  | `false`    | 差分とファイルタブで開いたファイルの長い行を、横にスクロールせずに折り返す                           |
| `branch-max-age` | `0` | ブランチの切り替えで、その週数コミットのないブランチを隠します（フィルタで見つけたものは表示）。`0` で隠しません |
| `worktree-base` | `default` | [セッション専用のワークツリー](./worktrees.md)で名前を付けて作る新しいブランチの起点: リポジトリのデフォルトブランチ `default`、またはプロジェクトのフォルダのブランチ `current` |
| `worktree-dir` |           | セッション専用のワークツリーを置く場所。それぞれリポジトリ名の下に置かれます。空ならリポジトリ内の `.kero/worktrees` |
| `worktree-from-origin` | `true` | セッション専用のワークツリーを作る前に元のブランチを取得し、リモートのブランチから作ります。取得に失敗したとき（オフラインなど）や、ローカルのブランチにリモートにないコミットがあるときは、ローカルのブランチから作ります |
| `worktree-branch-prefix` | `semantic` | [新しいワークツリーのタスクから](./sessions.md#new-session-dialog)名前を付けたブランチの接頭辞: 作業の種類（`feat/`、`fix/` など）の `semantic`、`worktree-branch-static-prefix` の `static`、`worktree-branch-prompt` の指示どおりの `custom` |
| `worktree-branch-static-prefix` | `kero/` | `worktree-branch-prefix` が `static` のときの接頭辞。`{type}` は作業の種類になります |
| `worktree-branch-prompt` |        | 新しいワークツリーのブランチ名をタスクから付ける方法。Kero のものの代わりに使い、行の間は `\n` |
| `worktree-cleanup-enabled` | `false` | Kero の起動時と 1 時間ごとに、下の条件のいずれかに当てはまる [Kero が作ったワークツリーを削除](./worktrees.md#cleaning-up-worktrees)します |
| `worktree-cleanup-inactive-days` | `30` | その日数、コミット、チェックアウト、セッションがないワークツリーを削除します。`0` で削除しません |
| `worktree-cleanup-merged` | `true` | ブランチの GitHub のプルリクエストが、ワークツリーのコミットでマージされたら削除します |
| `worktree-cleanup-unchanged` | `false` | デフォルトブランチ以外のコミットがないワークツリーを削除します |
| `worktree-cleanup-closed` | `false` | セッションがなくなったワークツリーを削除します |
| `worktree-cleanup-project` |  | プロジェクト独自の整理。フォルダごとに `<フォルダ>=off`、または `<フォルダ>=` に続けて `inactive:<日数>`、`merged`、`unchanged`、`closed` をカンマ区切りで。繰り返し指定できます |
| `commit-message-command` |  | ステージした変更の[コミットメッセージを書く](./changes.md#committing)コマンド。シェルで実行され、プロンプトを標準入力で受け取ります（`llm -m gpt-5` など）。中の `{session}` はフォークするペインのエージェントの会話の ID になります（`cc -p --resume {session} --fork-session` など）。空なら、ペインのエージェントの会話をフォークし、なければ Claude Code か Codex を新しく実行します |
| `commit-message-prompt` |  | エージェントがコミットメッセージを書く方法。最近のコミットとステージした差分の前に置く指示です。空なら Kero のもの（設定 › Git に表示）。改行は `\n` と書きます |
| `pr-message-prompt` |  | エージェントが[プルリクエスト](./pull-requests.md#new-pull-request)のタイトルと説明を書く方法。`commit-message-prompt` と同様です |
| `pr-notify` | `review` | プロジェクトの GitHub リポジトリの新しいプルリクエストのうち、[通知する](./pull-requests.md#pull-request-notifications)もの。`review` はレビューを依頼するもの、`all` は他の人が開いたものすべて、`off` はなし (確認も止まります) |
| `pr-poll-interval` | `5` | 新しいプルリクエストを確認する間隔 (分) |
| `pr-watch-wake` | `all` | [セッションのプルリクエスト](./pull-requests.md#a-session-s-pull-request)の何でセッションのエージェントを起こすか: `all` は失敗・成功したチェック、他の人のコメントとレビュー、マージコンフリクト。`failures` は失敗したチェックとマージコンフリクト。`off` は何もせず、見守りも止まります |
| `review-command` |  | リポジトリのプルリクエストを[レビュー](./pull-requests.md#reviewing-with-an-agent)するエージェントのコマンド（`owner/name=codex --full-auto` など）。空なら `claude`。エージェントはプロンプトを 1 つの引数として、`{prompt}` の位置に、なければコマンドの後に受け取ります。プルリクエストのプレースホルダーも使えます。リポジトリごとに繰り返します |
| `review-prompt` |  | レビューするエージェントへの指示（`owner/name=/code-review {ticket} #{number}` など）。空なら Kero のもの。改行は `\n` と書きます。リポジトリごとに繰り返します |
| `pr-review-prompt` |  | 自分の `review-prompt` がないリポジトリで、[レビュー](./pull-requests.md#reviewing-with-an-agent)するエージェントへの指示。空なら Kero のもの。改行は `\n` と書きます |
| `pr-guide-prompt` |  | プルリクエストの[レビューガイド](./pull-requests.md#review-guides)を書くエージェントへの指示。空なら Kero のもの。改行は `\n` と書きます |
| `repo-commit-message-prompt` |  | リポジトリ独自の `commit-message-prompt`（`owner/name=Write in Finnish` など）。リポジトリごとに繰り返します |
| `repo-pr-message-prompt` |  | リポジトリ独自の `pr-message-prompt`。`repo-commit-message-prompt` と同様です |
| `repo-merge-method` |  | リポジトリのプルリクエストを[マージ](./pull-requests.md#merging-and-other-actions)する既定の方法。`owner/name=rebase` のように、`merge`、`squash`、`rebase` のいずれか。なければ、スカッシュ、マージ、リベースのうちリポジトリが許す最初のもの |

## ファイル

| キー     | 既定値 | 説明                                                                                                                         |
| -------- | ------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `editor` | 空     | **エディタで開く**がファイルのパスを付けて実行するコマンド（`code`、`zed`、`"C:\Tools\editor.exe" -n` など）。空のときは、ファイルはシステムがそのファイルを開くアプリで、フォルダはテキストファイルを開くアプリで開きます |
| `files-hidden` | `true` | ファイルタブにドットファイルを表示する |

<a id="browser"></a>

## ブラウザ

| キー                  | デフォルト                 | 説明                                                                                |
| --------------------- | -------------------------- | ----------------------------------------------------------------------------------- |
| `browser-block-ads`   | `true`                     | ブラウザペインで[広告とトラッカーをブロック](./panes.md#blocking-ads)する           |
| `browser-filter-list` | `easylist`、`easyprivacy`  | ブロックに使うフィルタリスト。リストごとに繰り返す。空欄ならなし                    |
| `browser-devtools`    | `false`                    | Kero の起動時に、Playwright や chrome-devtools-mcp 向けの [DevTools ポート](./panes.md#driving-panes-with-devtools-tools)を `127.0.0.1` に開く。Windows のみ |
| `browser-devtools-port` | `9222`                   | `browser-devtools` が開くポート。`0` ならブラウザが選び、`kero browser devtools` が表示する |

`browser-filter-list` には、Kero が提供するリストの名前か、Adblock Plus 構文の任意のリストの `https://` URL を指定します:

| 名前               | リスト                                                              |
| ------------------ | ------------------------------------------------------------------- |
| `easylist`         | EasyList: 広告                                                      |
| `easyprivacy`      | EasyPrivacy: トラッカーとアクセス解析                               |
| `peter-lowe`       | Peter Lowe's list: 広告とトラッカーのサーバー                       |
| `easylist-china`   | EasyList China: 中国語サイトの広告                                  |
| `fanboy-cookie`    | EasyList Cookie List: Cookie の通知と同意バナー                     |
| `fanboy-social`    | Fanboy's Social Blocking List: ソーシャルメディアのボタンと埋め込み |
| `fanboy-annoyance` | Fanboy's Annoyance List: ポップアップと勧誘。上の 2 つも含む        |

```ini
browser-filter-list = easylist
browser-filter-list = easyprivacy
browser-filter-list = fanboy-cookie
browser-filter-list = https://example.com/my-filters.txt
```

<a id="keyboard"></a>

## キーボード

`keybind` は `<keys>=<action>` の形でアクションの割り当てを変えます。割り当てごとに繰り返します。キーを空けるには、アクションに `unbind` を使います。

キーは、修飾キーとキーを `+` でつないで書きます。`cmd`（または `super`、`command`）は macOS では ⌘、Windows と Linux では <kbd>Ctrl</kbd> です。`ctrl`（`control`）は <kbd>Ctrl</kbd>、`alt`（`opt`、`option`）は <kbd>Alt</kbd> または <kbd>⌥</kbd>、そして `shift` です。キーは文字、数字、記号、または `enter`（`return`）、`tab`、`space`、`backspace`、`delete`、`escape`（`esc`）、`up`、`down`、`left`、`right`、`home`、`end`、`pageup`、`pagedown`、そして <kbd>+</kbd> を表す `plus` です。そのため、下の `cmd+shift+d` は Windows では <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>D</kbd> です。

```ini
keybind = cmd+shift+d=pane.split.down
keybind = cmd+k=palette.open
keybind = cmd+b=unbind
```

すべてのアクション ID は `kero actions` で確認できます。デフォルトは[キーボードショートカット](./shortcuts.md)を参照してください。
