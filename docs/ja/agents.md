---
title: コーディングエージェント
description: Claude Code、Codex などのエージェントに、自分の横で作業を動かし、Kero に進捗を報告することを教えます。
---

# コーディングエージェント

Kero で動くエージェントは、あなたと同じように `kero` コマンドを使えます: 下のペインで開発サーバーを起動し、2 つ目のエージェントにレビューを任せ、テストを待ち、行き詰まったらあなたに知らせます。

<a id="install-the-skill"></a>

## スキルをインストールする

```sh
kero skill setup
```

これで 2 つのものがインストールされます:

- エージェントに CLI を教える **スキル**。Claude Code は `~/.claude/skills/kero` から、Codex、OpenCode などのエージェントは `~/.agents/skills/kero` から読み込みます。
- エージェントがセッションを始めるときに実行される **フック**。エージェントに、どの Kero セッションとペインにいるかを伝えるので、それを調べる手間が省けます。また、名前のないセッションに `fix-login-redirect` のようなタスクにちなんだ名前を付けさせるので、あなたが付ける必要はありません。さらに、エージェント自身のセッションを Kero に伝えるので、再起動のあとなど kerod が再起動したとき、エージェントは[会話を再開](./sessions.md#kerod)します。

フックは、インストールされているエージェントごとに、それぞれが探す場所に置かれます:

| エージェント | フックの場所                                          |
| ------------ | ----------------------------------------------------- |
| Claude Code  | `~/.claude/settings.json` の `SessionStart` フック     |
| Codex        | `~/.codex/hooks.json` の `SessionStart` フック         |
| Pi           | 拡張機能 `~/.pi/agent/extensions/kero.ts`              |
| OMP          | 拡張機能 `~/.omp/agent/extensions/kero.ts`             |
| OpenCode     | プラグイン `~/.config/opencode/plugins/kero`           |

`CLAUDE_CONFIG_DIR`、`CODEX_HOME`、`PI_CODING_AGENT_DIR`、`PI_CONFIG_DIR`、`XDG_CONFIG_HOME` でエージェントのファイルの場所が変わっている場合、Kero はそれに従い、各設定ファイルのほかの部分はそのままにします。

1 つのエージェントだけをセットアップするには、`--agent` に `claude`、`codex`、`pi`、`omp`、`opencode` のいずれかを渡します。セットアップは一度だけです: スキルは Kero 自身のコピーへのリンクで、Kero のアップデートに合わせて更新されます。Pi、OMP、OpenCode のプラグインも Kero が最新に保ちます。`kero skill status` で、各部分が最新かどうかがわかります。

```sh
kero skill status    # インストール済みのものと、それが最新かどうか
kero skill show      # スキルを出力
kero skill remove    # スキルとフックをアンインストール
```

フックが働くのはセットアップ後に始めたエージェントのセッションだけで、エージェントが先に確認を求めることもあります:

- **Claude Code** は、信頼したフォルダでのみフックを実行します。
- **Codex** は新しいフックの承認を求めます: `/hooks` で Kero のフックを承認してください。
- **OpenCode** は、バックグラウンドサービスの起動時にプラグインを読み込みます: 作業中のセッションがないときに、一度 `opencode service restart` を実行してください。

OpenCode はエージェントのコマンドをペインではなくバックグラウンドサービスで実行するので、`KERO_SESSION` と `KERO_PANE` がありません。代わりに Kero がエージェントにペインを伝え、エージェントはそれを対象として指定します。同じ理由で、OpenCode のエージェントからの `kero report` はそのペインに届きません。

Kero の外では、フックは何もしません。

## エージェントにできること

自分の横で何かを動かす:

```sh
kero split --down -- npm run dev
kero new -n review -- claude
```

それを操作して結果を読む:

```sh
kero prompt review "review the changes in src/auth"
kero wait tests --for idle,error --timeout 10m
kero read tests -n 60
```

自分の横でテストを実行して、失敗した内容を読む:

```sh
pane=$(kero split --down -- go test ./...)
kero wait "$pane" --exit || kero read "$pane" -n 80
```

`git worktree list` にはない情報つきでリポジトリのワークツリーを見る（各ワークツリーのセッション、Kero が作ったか、削除で失うもの、自動クリーンアップがどうするか）。自分のセッションのワークツリー、セットアップ、プルリクエストを確かめる。自分のセッションを会話ごと新しい git ワークツリーに移し、ターンの終わりにそこで続ける（[詳しく](./worktrees.md#moving-a-session-into-a-worktree)）:

```sh
kero worktree list
kero worktree status
kero worktree handoff fix-login --prompt "go on with the fix"
```

プルリクエストを扱う: [レビューガイド](./pull-requests.md#review-guides)を書き、セッションにリンクされたプルリクエストを見ます。Kero はそれを見守り、チェックが失敗すると「Kero PR watch:」で始まるメッセージをエージェントのプロンプトに入力します。頼まれれば、マージ、更新、チェックアウトもします（[すべて](./cli.md)）:

```sh
kero pr guide stop src/queue.go:10-42 --title "The new queue" "Why it exists."
kero pr status                  # リンクされたプルリクエスト: チェック、コンフリクト
kero pr update-branch --rebase
kero pr merge --squash          # 頼まれたときだけ: マージは取り消せません
kero pr checkout 123 --worktree -- claude
```

状況をあなたに伝える:

```sh
kero report working "Migrating the database" --progress 40
kero report blocked "Approve the production deploy?"
kero report done "Deployed to staging"
```

`blocked` の報告でセッションは*待機中*になり、あなたに通知されます。エージェントがこれを使うのは、あなたなしでは続けられないときだけにしてください。

## ほかのエージェントを動かすエージェント

- [オーケストレーター](./orchestrators.md)は、作業を計画し、そのための子セッションを開始して見守るエージェントセッションです。あなたが話しかけるのはオーケストレーターだけです。
- [Ama](./ama.md) は Kero のヘルパーエージェントです。あなたの代わりにセッション、ワークツリー、プロジェクト、設定を開始・管理し、Kero についての質問に答えます。

<a id="remote-control"></a>

## リモート操作

**リモート操作** がオン（デフォルト、**設定 › エージェント**）なら、エージェントはアプリ自体も操作できます: `kero focus` でセッションを表示し、`kero do` でメニューのアクションを実行し、`kero browser` でブラウザペインを操作し、`kero pr review` でプルリクエストのレビューを始めます。エージェントを自分のセッションだけにとどめるには、`remote-control = false` でオフにします。[Ama](./ama.md#without-remote-control) には不要です。

## 環境変数

すべてのペインには次の変数があります:

| 変数           | 値                             |
| -------------- | ------------------------------ |
| `KERO_SESSION` | ペインのセッションの ID        |
| `KERO_PANE`    | ペインの ID                    |
| `KERO_SOCKET`  | kerod が待ち受ける場所。`kero` が kerod に接続するために使います |
| `TERM`、`COLORTERM` | `xterm-256color` と `truecolor` |
| `TERM_PROGRAM`、`TERM_PROGRAM_VERSION` | `ghostty` と Ghostty のバージョン。Kero のターミナルは Ghostty のものなので、Ghostty のキーや通知を知るプログラムはそれを使います |
| `LC_TERMINAL`、`LC_TERMINAL_VERSION` | `kero` と、そのプロトコルのバージョン |
| `SHELL`        | ペインのシェル。Windows では、bash のような POSIX シェルのときだけ |
| `LANG`         | kerod に `LANG`、`LC_ALL`、`LC_CTYPE` のどれもなかったときは `en_US.UTF-8` |
| `PATH`         | kerod のもので、末尾に `kero` のある Kero 自身の `bin` フォルダが付きます |

`env` 設定の変数はこれらの後に設定され、優先されます。ペインは、kerod が起動したときの環境（アプリの環境）の残りも受け取ります。ただし、`TMUX`、`VSCODE_*`、`GHOSTTY_*`、`CLAUDECODE`、`CLAUDE_CODE_*` のように、ほかのターミナルやエージェントのものは除きます。そのため、Kero は `NO_COLOR` のないシェルから起動してください。そうしないと、すべてのペインで色が出なくなります（[トラブルシューティング](./troubleshooting.md#panes-without-colors)）。[Ama](./ama.md) のペインには `KERO_AMA` も渡され、ワークツリーの[セットアップスクリプト](./worktrees.md#setup-scripts)には `KERO_WORKTREE_PATH`、`KERO_PROJECT_ROOT`、`KERO_WORKTREE_BRANCH` が渡されます。

スクリプトが Kero の中で動いているかどうかは、`$KERO_PANE` で確認できます。
