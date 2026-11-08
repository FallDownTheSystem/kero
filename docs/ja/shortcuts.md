---
title: キーボードショートカット
description: Kero のデフォルトのキーボードショートカットと、割り当てを変えるためのアクション ID。
---

# キーボードショートカット

どれも **設定 › キーボード**、または[設定ファイル](./configuration.md#keyboard)の `keybind` で割り当てを変えられます。

Windows と Linux では、<kbd>Ctrl</kbd> と文字キーの組み合わせはシェルのもの（<kbd>Ctrl</kbd> <kbd>D</kbd>、<kbd>Ctrl</kbd> <kbd>R</kbd>、<kbd>Ctrl</kbd> <kbd>W</kbd>…）なので、Kero のショートカットは Windows Terminal と同じく <kbd>Shift</kbd> を加えるか、<kbd>Alt</kbd> を使います。ダッシュは、そのアクションにショートカットがないことを示します。

## Kero

| アクション                       | macOS                                  | Windows と Linux                              | アクション ID    |
| -------------------------------- | -------------------------------------- | --------------------------------------------- | ---------------- |
| 設定…                            | <kbd>⌘</kbd> <kbd>,</kbd>              | <kbd>Ctrl</kbd> <kbd>,</kbd>                  | `settings.open`  |
| 終了（セッションは動き続ける）   | <kbd>⌘</kbd> <kbd>Q</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Q</kbd> |                  |
| 終了してすべてのセッションを終える… | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>Q</kbd> | –                                          | `app.quitAll`    |
| コマンドラインツールをインストール… | –                                   | –                                             | `cli.install`    |

`kero actions` は、`kero do` が実行するアクションを、このコンピュータでのキーとともに一覧します。ここにあるほかのアクション（`app.quitAll`、`session.rename`、`find.open` など）はキーを割り当てられますが、`kero do` では実行できません（[アプリのアクションを実行する](./cli.md#running-app-actions)）。

## 編集

| アクション | macOS                                  | Windows と Linux                              | アクション ID   |
| ---------- | -------------------------------------- | --------------------------------------------- | --------------- |
| 検索…      | <kbd>⌘</kbd> <kbd>F</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>F</kbd> | `find.open`     |
| 次を検索   | <kbd>⌘</kbd> <kbd>G</kbd>              | –                                             | `find.next`     |
| 前を検索   | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>G</kbd> | –                                             | `find.previous` |

検索は今いるペインを検索します。ビューアではそのファイルや差分を検索し、右サイドバーのツリーでは[検索](./files.md#search)を開きます。

## セッション

| アクション                   | macOS                                  | Windows と Linux                              | アクション ID       |
| ---------------------------- | -------------------------------------- | --------------------------------------------- | ------------------- |
| 新規セッション               | <kbd>⌘</kbd> <kbd>T</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>T</kbd> | `session.new`       |
| ホームで新規セッション       | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>T</kbd> | –                                             | `session.newHome`   |
| ワークツリーで新規セッション… | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>T</kbd> | –                                             | `session.newWorktree` |
| プロジェクトを開く…          | <kbd>⌘</kbd> <kbd>O</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>O</kbd> | `project.open`      |
| セッションへ移動…            | <kbd>⌘</kbd> <kbd>P</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>P</kbd> | `palette.open`      |
| ファイルへ移動…              | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>N</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>N</kbd> | `files.goto`        |
| 会話を再開…                  | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>O</kbd> | –                                             | `conversation.resume` |
| 会話を検索…                  | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>F</kbd> | –                                             | `conversation.search` |
| 対応が必要なセッションへ移動 | <kbd>⌘</kbd> <kbd>J</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>J</kbd> | `session.attention` |
| 次のセッション               | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>]</kbd> | <kbd>Ctrl</kbd> <kbd>PgDn</kbd>               | `session.next`      |
| 前のセッション               | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>[</kbd> | <kbd>Ctrl</kbd> <kbd>PgUp</kbd>               | `session.previous`  |
| セッション名を変更           | <kbd>⌘</kbd> <kbd>R</kbd>              | –                                             | `session.rename`    |
| ペインを再起動               | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>R</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>R</kbd> | `pane.restart`      |
| 閉じる                       | <kbd>⌘</kbd> <kbd>W</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>W</kbd> | `pane.close`        |
| セッションを閉じる           | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>W</kbd> | –                                             | `session.close`     |

<kbd>⌘</kbd> <kbd>1</kbd>–<kbd>9</kbd>（Windows と Linux では <kbd>Ctrl</kbd> <kbd>1</kbd>–<kbd>9</kbd>）でサイドバーのセッションへ、<kbd>Ctrl</kbd> <kbd>Tab</kbd> で次のセッションへ移動します。

## ペイン

| アクション         | macOS                                  | Windows と Linux                                  | アクション ID      |
| ------------------ | -------------------------------------- | ------------------------------------------------- | ------------------ |
| 右に分割           | <kbd>⌘</kbd> <kbd>D</kbd>              | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>=</kbd>      | `pane.split.right` |
| 下に分割           | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>D</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>-</kbd>      | `pane.split.down`  |
| 新規ブラウザペイン | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>B</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>B</kbd>      | `pane.browser`     |
| 場所を開く         | <kbd>⌘</kbd> <kbd>L</kbd>              | –                                                 | `browser.address`  |
| 要素を選ぶ | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>C</kbd> | <kbd>Alt</kbd> <kbd>Shift</kbd> <kbd>C</kbd> | `browser.pick` |
| ペインを拡大       | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>↩</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Enter</kbd> | `pane.zoom`        |
| Markdownプレビューの切り替え | –                            | –                                                 | `pane.preview`     |
| 次のペイン         | <kbd>⌘</kbd> <kbd>]</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>]</kbd>     | `pane.next`        |
| 前のペイン         | <kbd>⌘</kbd> <kbd>[</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>[</kbd>     | `pane.previous`    |
| 左のペイン         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>←</kbd> | <kbd>Alt</kbd> <kbd>←</kbd>                       | `pane.left`        |
| 右のペイン         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>→</kbd> | <kbd>Alt</kbd> <kbd>→</kbd>                       | `pane.right`       |
| 上のペイン         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↑</kbd> | <kbd>Alt</kbd> <kbd>↑</kbd>                       | `pane.up`          |
| 下のペイン         | <kbd>⌘</kbd> <kbd>⌥</kbd> <kbd>↓</kbd> | <kbd>Alt</kbd> <kbd>↓</kbd>                       | `pane.down`        |

<a id="view"></a>

## 表示

| アクション             | macOS                                  | Windows と Linux                              | アクション ID    |
| ---------------------- | -------------------------------------- | --------------------------------------------- | ---------------- |
| サイドバーの切り替え   | <kbd>⌘</kbd> <kbd>B</kbd>              | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>B</kbd> | `sidebar.toggle` |
| 右サイドバーの切り替え | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>B</kbd> | –                                             | `rightSidebar.toggle` |
| 変更の切り替え         | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>G</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>G</kbd> | `changes.toggle` |
| ファイルの切り替え     | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>E</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>E</kbd> | `files.toggle`   |
| 履歴の切り替え         | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>Y</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>Y</kbd> | `history.toggle` |
| 検索の切り替え         | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>F</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>S</kbd> | `search.toggle`  |
| レビューの切り替え     | <kbd>⌥</kbd> <kbd>⌘</kbd> <kbd>P</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>K</kbd> | `review.toggle`  |
| 変更、ファイル、履歴、検索を表示 | –                            | –                                             | `changes.show`、`files.show`、`history.show`、`search.show` |
| 拡大                   | <kbd>⌘</kbd> <kbd>=</kbd>              | <kbd>Ctrl</kbd> <kbd>=</kbd>                  | `text.bigger`    |
| 縮小                   | <kbd>⌘</kbd> <kbd>-</kbd>              | <kbd>Ctrl</kbd> <kbd>-</kbd>                  | `text.smaller`   |
| 実際のサイズ           | <kbd>⌘</kbd> <kbd>0</kbd>              | <kbd>Ctrl</kbd> <kbd>0</kbd>                  | `text.reset`     |
| ダークモードの切り替え | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>L</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>L</kbd> | `theme.toggle` |
| クイックターミナル     | –                                      | –                                             | `quickTerminal.toggle` |
| Ama            | <kbd>⌘</kbd> <kbd>⇧</kbd> <kbd>A</kbd> | <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>A</kbd> | `ama.toggle` |

切り替えのアクションは右サイドバーのタブを表示し、表示中なら隠します。表示のアクションは表示するだけで、`kero do` がエージェントのために実行するのはこちらです。

<a id="the-command-palette"></a>

## コマンドパレット

**セッションへ移動…**（<kbd>⌘</kbd> <kbd>P</kbd>、Windows と Linux では <kbd>Ctrl</kbd> <kbd>Shift</kbd> <kbd>P</kbd>）はコマンドパレットです: 入力して、セッションを名前で探したり、再開する最近の会話を探したり、コマンドを実行したりします。コマンドには、すべてのメニューのアクション、設定のページ（**設定: Git**）、**バックグラウンドサービスを再起動**、コンピュータをスリープさせないこと、各プロジェクト・各ホストでの新規セッション、そして Web アドレスなら **ブラウザペインで開く** があります。コマンドはどの言語でも英語の名前で見つかります。

<a id="quick-terminal"></a>

## クイックターミナル

クイックターミナルは、Ghostty のクイックターミナルや Quake 風のコンソールのように、どのアプリの上でも画面の上端から降りてきて、ショートカットをもう一度押すか別の場所をクリックすると戻ります。ショートカットはどのアプリでも効くため、**設定 › キーボード** か[設定ファイル](./configuration.md#keyboard)で割り当てるまでは何も割り当てられていません:

```ini
keybind = ctrl+`=quickTerminal.toggle
```

専用のセッション「クイックターミナル」を表示し、隠している間も Kero を終了した後も動き続け、サイドバーにも他のセッションと一緒に表示されます。シェルが終了すると、次に押したときに新しいシェルが始まります。**表示 › クイックターミナル** やメニューバーのアイコンのメニューからも開け、`kero do quickTerminal.toggle` でスクリプトから切り替えられます。
