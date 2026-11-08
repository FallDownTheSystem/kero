---
title: トラブルシューティング
description: Kero がファイルとログを置く場所、kerod の再起動、アップデートが失敗したときの対処、各部分の確認方法。
---

# トラブルシューティング

<a id="where-kero-keeps-things"></a>

## Kero がファイルを置く場所

Kero は、設定を 1 つのフォルダに、自分の状態を別のフォルダに置きます。

**設定フォルダ**は `~/.config/kero`（または `$XDG_CONFIG_HOME/kero`）です。Windows でも同じで、`C:\Users\<you>\.config\kero` になります。`kero config path` で設定ファイルのパスを表示します。

| ファイル | 内容 |
| --- | --- |
| `config` | あなたの[設定](./configuration.md) |
| `agents.toml` | [独自のエージェント](./status.md#your-own-agents)（ある場合） |
| `ama/` | [Ama のフォルダ](./ama.md#its-folder): Ama への指示、あなたの `USER.md`、Kero のドキュメント |

**データフォルダ**は、Windows では `%AppData%\kero`、macOS では `~/Library/Application Support/kero`、Linux では `~/.config/kero` で、Linux では設定フォルダも兼ねます。`$KERO_HOME` で場所を変えられます。**設定 › 詳細** に、その場所とログが表示されます。

| ファイル | 内容 |
| --- | --- |
| `kerod.log` | kerod の[ログ](#logs) |
| `state.json` | プロジェクトとセッション。kerod は再起動後にこれをもとに再開します |
| `sessions/` | 各ペインのスクロールバック |
| `conversations.json` | エージェントの[過去の会話](./sessions.md#past-conversations) |
| `app.json`、`window-state.json` | ウインドウ: サイズと位置、サイドバー、Ama のウインドウ |
| `updater.json` | Kero が最後にアップデートを確認した日時と、スキップしたバージョン |
| `reviews/`、`guides/` | [下書きのレビュー](./pull-requests.md#draft-reviews)と[レビューガイド](./pull-requests.md#review-guides)。プルリクエストごとに 1 ファイル |
| `worktree-cleanup.json` | [ワークツリーの整理](./worktrees.md#cleaning-up-worktrees)がワークツリーについて把握していること |
| `skills/kero/` | Kero のスキル。エージェントのスキルはここにリンクします |
| `adblock/` | [広告ブロック](./panes.md#blocking-ads)のフィルタリスト |
| `bin/` | ペインが `PATH` で見つける `kero` コマンド |
| `kerod.sock`、`kerod.lock`、`kerod.running` | アプリと `kero` が kerod に接続する方法と、kerod が動いているかどうか。`$KERO_SOCKET` でソケットの場所を変えられます |

Windows では、ブラウザペインは Cookie とサイトのデータを `%LOCALAPPDATA%\Kero\WebView2` に保存します。

Kero が作るワークツリーは、各リポジトリの `.kero/worktrees`、または `worktree-dir` の下に置かれます（[ワークツリー](./worktrees.md#where-worktrees-go-and-what-they-start-from)）。

<a id="logs"></a>

## ログ

kerod はデータフォルダの `kerod.log` に 1 つのログを書きます。Windows では `%AppData%\kero\kerod.log` です。**設定 › 詳細 › ログ** で、ファイルマネージャーに表示できます。アプリや `kero` が kerod を起動したとき、kerod が起動時に出力するもの（起動に失敗したときも）もここに入ります。

kerod の各実行は `--- daemon run ---` と、バージョンとプロセスを示す行から始まります。ログには、セッションの開始、終了、再起動後の復帰、ワークツリーの作成と削除、`.worktreeinclude` のコピーと[セットアップスクリプト](./worktrees.md#setup-scripts)（`setup script of <worktree> runs in pane <pane>: <command>` と、その後に準備がどう終わったか）、`agents.toml` の問題（`agents: …`）、エラーが記録されます。`previous daemon exit was unclean` は、コンピュータの電源が落ちたときのように、kerod が前回保存せずに終了したことを示します。ログが 4 MB を超えると、Kero は kerod の起動時にログを空にします。

`kero daemon serve` は、ログを標準エラーにも書きます。

<a id="restarting-kerod"></a>

## kerod の再起動

kerod は、セッションを動かすバックグラウンドサービスです（[kerod](./sessions.md#kerod)）。アップデートの後、アプリが別のバージョンだと示したとき、応答しなくなったときに再起動します:

- サイドバーの **再起動** ボタン。Kero がアップデートされると「Kero がアップデートされました: 新しいバージョンを使うには、バックグラウンドサービスを再起動してください」と表示されます。
- **設定 › 詳細 › バックグラウンドサービス › 再起動**。
- コマンドパレットの **バックグラウンドサービスを再起動**。
- `kero daemon restart`。

再起動は先にセッションを保存し、スクロールバックとともに元の場所で再開します。エージェントは会話を再開します。エージェント以外のプログラムはシェルに戻ります。

```sh
kero daemon status       # kerod が動いているか: プロセス、稼働時間、セッションとペイン、ソケット
kero daemon restart
kero daemon stop         # 停止する。次に kerod が起動するとセッションも再開する
```

`kero` は kerod に接続できないとき、終了ステータス `4` で終了します。アプリのサイドバーには **kerod に接続できません** と表示されます。アプリを開くか、何か `kero` コマンドを実行すると、kerod が再び起動します。kerod と別のバージョンの `kero` は、`kero daemon restart` を実行するよう表示します。

kerod は、セッションも接続もない状態が 30 秒続くと自分で終了します。ただし、サービスとして動いているとき（`kero daemon serve`）やほかのマシンからの接続を待ち受けているときは終了しません。ログには `idle: exiting` と記録されます。

## アップデート

Kero は開いている間、1 日に 1 回アップデートを確認し、**設定 › 一般 › アップデートを確認** で今すぐ確認します。**アップデートを自動的にダウンロードしてインストール** は、アップデートが来るとインストールし、次に Kero を開いたときに有効になります。

**確認に失敗しても**（GitHub からの `504 Gateway Timeout` など）、何も変わりません: 自動の確認は何も言わずに失敗して後でもう一度試し、**アップデートを確認** は確認できなかったことを表示します。500 から 504 のステータスは GitHub 側の問題で、あなたの側ではありません: 後でもう一度試してください。今すぐアップデートするには、手でインストールします:

1. Kero のアップデート元のリポジトリのリリースページからインストーラ（Windows では `kero-setup-<version>-amd64.exe`）をダウンロードします。このビルドでは [github.com/FallDownTheSystem/kero/releases](https://github.com/FallDownTheSystem/kero/releases) です。
2. `Kero.exe` を使用中にしている kerod を止めます: **終了してすべてのセッションを終える…**、または `kero daemon stop`。
3. インストーラを、Kero のペインやエージェントのシェルの中ではなく、エクスプローラーか自分のターミナルから実行します（[理由](#panes-without-colors)）。

アップデートの後は、セッションが新しいバージョンで動くよう、サイドバーの案内どおりに kerod を再起動します。ホストの Kero は `kero -H <host> daemon update` か `kero hosts install <host>` でアップデートします（[リモートホスト](./remote.md#versions-and-updates)）。

<a id="panes-without-colors"></a>

## 色のないペイン

kerod は、起動したときの環境をすべてのペインに渡します。エージェントのシェルによくあるように `NO_COLOR` を設定するシェルから起動すると、すべてのペインのすべてのプログラムが色を出さなくなります。**終了してすべてのセッションを終える…** か `kero daemon stop` で Kero を終了し、スタートメニュー、Dock、または自分のターミナルからもう一度開いてください。シェルが自分のために設定したほかの変数も同じです。

## エージェント

- **エージェントの状態が表示されない、セッションに名前が付かない、再起動後に会話を再開しない**: そのエージェント向けに Kero のフックがセットアップされていません。`kero skill status` でインストール済みで最新のものがわかり、`kero skill setup` でインストールします（[スキルをインストールする](./agents.md#install-the-skill)）。フックが働くのはセットアップ後に始めたセッションだけです。Claude Code は信頼したフォルダでのみフックを実行し、Codex は `/hooks` での承認を求めます。
- **エージェントの `kero do`、`kero focus`、`kero browser` が失敗する**: これらには **リモート操作**（**設定 › エージェント**、`remote-control`）と、アプリが開いていることが必要です。`kero browser wait-message`、`kero browser devtools`、`kero do pane.preview` は不要です。[Ama](./ama.md#without-remote-control) にはリモート操作は不要です。
- **新規セッションのダイアログでエージェントのコマンドが見つからない**: Kero はあなたと同じようにシェルでエージェントを実行するので、エイリアスはそれを定義するシェルでしか働きません。`kero config get agent` でエージェントを一覧し、`kero config check` で `agents.toml` の誤りを報告します。

## ワークツリー

- **セットアップスクリプトが実行されなかった**: 実行されるのは Kero が作るワークツリーだけで、`kero.json` か `t3.json` の `"runOnWorktreeCreate": true` を持つ最初のスクリプトだけです（[セットアップスクリプト](./worktrees.md#setup-scripts)）。ワークツリーのセッションで `kero worktree status` を実行すると準備の様子がわかり、`kerod.log` には何が実行され、どう終わったかが記録されます。有効な JSON でない `kero.json` は準備を失敗させます。
- **新しいワークツリーに無視されたファイルがない**: メインのチェックアウトの `.worktreeinclude` に並べます（[無視されたファイルのコピー](./worktrees.md#copying-ignored-files)）。
- **ワークツリーが削除されない**: `kero worktree list` で、各ワークツリーを削除すると失うものと、整理がそれをどう判断するかがわかります。整理は、変更のあるワークツリー、セッションが作業中のワークツリー、作って 10 分以内のワークツリーを決して削除しません。

## 設定

`kero config check` は、設定ファイルと `agents.toml` の不明なキーと誤った値を報告します。値を付けない `kero config set <key>` はそのキーをデフォルトに戻し、**設定 › 詳細 › すべての設定をリセット** はキーボードショートカットも含めてすべてのキーを戻します。セッションはそのまま残ります。

## 各部分の確認

```sh
kero version                 # kero とそのプロトコルのバージョン
kero daemon status           # kerod
kero clients                 # kerod に接続しているアプリとコマンド
kero config check            # 設定ファイルと agents.toml
kero skill status            # エージェントごとのスキルとフック
kero worktree status         # このセッションのワークツリー、準備、プルリクエスト
kero hosts check <host>      # ホストに接続して、結果を表示する
kero browser devtools        # ブラウザペインの DevTools のポート
kero ls --json               # すべてのセッションとペインと、その状態
```
