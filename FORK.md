# Maintaining this fork

This repository is FallDownTheSystem/kero, a fork of
[egoist/kero](https://github.com/egoist/kero). It keeps a few features of
its own (the Files view, editor panes, the Files toolbar and CLI additions,
ending all sessions from the app icon's menu, the Explorer and split-menu
layout and its settings, theme previews, the theme editor and the files'
own theme) as commits
on top of upstream's `main`, and is built and
installed on Windows only.

## Remotes and branches

- `origin`: the fork, `https://github.com/FallDownTheSystem/kero.git`.
- `upstream`: `https://github.com/egoist/kero`.
- `main`: upstream's `main` plus the fork's commits, always rebased, never
  merged, so that `git log upstream/main..main` lists exactly what the fork
  adds.
- `backup/pre-rebase-<version>`: `main` as it was before a rebase, to go
  back to if one goes wrong. Delete old ones once the next sync works.

## Syncing with a new upstream release

1. Start clean: `git status` shows nothing to commit.
2. Fetch, and see what's new and what the fork adds:

   ```sh
   git fetch upstream --tags
   git log --oneline main..upstream/main
   git log --oneline upstream/main..main
   ```

3. Back up, then rebase:

   ```sh
   git branch -f backup/pre-rebase-X.Y.Z main
   git rebase upstream/main
   ```

4. Resolve conflicts (see below), then check that it still builds and
   passes:

   ```sh
   go tool mygo generate
   go build ./...
   go vet ./...
   go tool mygo i18n check
   go test ./...
   ```

   On Windows these tests fail on upstream too, so they are not the fork's:
   TestPasteImageAsPath, TestPasteImageError, TestHookInstallKeepsSettings,
   TestAgentPlugins, TestWrite. Any other failure is.
5. Push, replacing the fork's `main` (and the backup, for safety):

   ```sh
   git push --force-with-lease origin main
   git push origin backup/pre-rebase-X.Y.Z
   ```

6. Build and install it (below).

## Where conflicts come from

- `locales/*.json`: both sides add keys. Keep both; the order of keys
  doesn't matter. Never resolve `locales/catalog_gen.go` by hand: take
  either side, then run `go tool mygo generate`, which writes it from the
  JSON.
- `go.mod` and `go.sum`: take upstream's MyGo version, then `go mod tidy`.
- `docs/` in three languages (en, ja, zh): keep both sides' paragraphs.
- `internal/actions/actions.go`, `actions.go`: both sides add actions; keep
  both, in the menus' order.
- `mygo.json`: always upstream's. Its version stays upstream's, so that the
  fork reports the version it is based on.

`go tool mygo generate` may leave `locales/catalog_gen.go` changed in line
endings only (`git diff` shows nothing): `git checkout -- locales/catalog_gen.go`.

## Rules for the fork's changes

- Change Kero, never MyGo. A MyGo bug is worked around in Kero, with a
  comment that says what MyGo does and when the workaround can go (as
  `padMenus` in `open_windows.go`), and is reported upstream.
- Keep the fork's commits few and whole: one per feature, each building
  and passing on its own, so that a rebase replays them cleanly. Fixes to
  a fork feature can be squashed into its commit when syncing
  (`git rebase -i` by hand).
- Follow upstream's style (see CLAUDE.md and the code around a change):
  tabs, CRLF, comments that say why, all three languages for text and docs.
- `proto.Version` stays upstream's: the fork adds only optional fields and
  new methods (`fs.create`, `pane.file`), which older apps and kerods
  ignore; an app that calls one on an older kerod falls back, as
  `replaceFile` opens a new pane.

## Building and installing

```sh
go tool mygo build -platform windows/amd64
```

leaves `build/windows-amd64/Kero Setup X.Y.Z amd64.exe`. Before running it:

- Turn off **Download and install updates automatically** in Settings ›
  General. The fork's build checks egoist's releases for updates, and with
  this on, the next official release replaces the fork's build.
- Stop kerod, which keeps running after the app quits and holds `Kero.exe`:
  quit with **End All Sessions and Quit** (the tray icon's menu, or the Kero
  menu), or run `kero daemon stop`. Both end the sessions.
- Start the installer from Explorer, or a normal terminal. The installer
  starts Kero when it is done, and Kero and kerod keep the environment it
  had: started from a shell that sets `NO_COLOR` (as an agent's does),
  every pane loses its colors.

The installer is not signed: SmartScreen doesn't ask about a file built on
this machine, but a work machine's security tools may still flag it.
