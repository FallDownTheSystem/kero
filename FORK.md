# Maintaining this fork

This repository is FallDownTheSystem/kero, a fork of
[egoist/kero](https://github.com/egoist/kero). It keeps a few features of
its own (the Files view, editor panes, the Files toolbar and CLI additions,
ending all sessions from the app icon's menu, the Explorer and split-menu
layout and its settings, theme previews, the theme editor and the files'
own theme, and git in the Explorer: staging and committing, branches,
syncing, stashes, merges and conflicts, and the History tab) as commits
on top of upstream's `main`, and is built and
installed on Windows only. It releases its Windows build itself, which
its installs update to.

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

6. Release it (below).

## Releasing the fork

A fork release is a tag `vX.Y.Z-fork.N`: X.Y.Z is `mygo.json`'s version,
upstream's, and N counts the fork's releases of it from 1. Pushing the tag
runs the Fork release workflow (`.github/workflows/fork-release.yml`),
which tests, builds the Windows app (`windows/amd64`) and publishes it as
the fork's latest release. Installed fork builds find it there: in the
runner, the workflow sets `mygo.json`'s version to the tag's, its
`updates` to the fork's repository and key, and its release notes to
`FORK-CHANGELOG.md`, so none of that is in the repository's `mygo.json`.

1. Add the version's section to the top of `FORK-CHANGELOG.md`, as
   `## 2.1.18-fork.2`: what the fork changed since its last release, and
   the upstream version, when it's new. The build stops without it.
2. Commit it, tag, and push:

   ```sh
   git commit -am "Release 2.1.18-fork.2"
   git tag v2.1.18-fork.2
   git push --force-with-lease origin main
   git push origin v2.1.18-fork.2
   ```

3. Watch it:

   ```sh
   gh run list --repo FallDownTheSystem/kero --workflow fork-release.yml --limit 1
   gh run watch <run id> --repo FallDownTheSystem/kero --exit-status
   ```

The `-fork.N` versions keep the fork's releases apart from upstream's,
which installs of the fork never update to. They are newer than one
another, and than the last upstream version's (2.1.19-fork.1 after
2.1.18-fork.3), but older than that version itself: a build made from
`mygo.json` alone, as `2.1.18`, takes no update to `2.1.18-fork.N`, so
install the release by hand once. Hosts (Settings › Hosts) aren't updated
from a fork build (`build.Fork`), as egoist/kero's server builds have no
`-fork` version: update them with `kero hosts install`.

The fork signs its updates with its own key, as `mygo keygen` made it: the
public key is in the workflow, the secret is the repository's
`MYGO_UPDATER_PRIVATE_KEY`, and a copy is kept outside the repository.
Without that key, installed fork builds take no update again.

Upstream's Release workflow is disabled in the fork
(`gh workflow disable release.yml`), as the fork's `v*` tags would start
it, and it builds for macOS with upstream's certificates.

## Where conflicts come from

- `locales/*.json`: both sides add keys. Keep both; the order of keys
  doesn't matter. Never resolve `locales/catalog_gen.go` by hand: take
  either side, then run `go tool mygo generate`, which writes it from the
  JSON.
- `go.mod` and `go.sum`: take upstream's MyGo version, and keep the fork's
  `replace` of MyGo. When upstream's version is a new one, rebase MyGo's
  `kero` branch onto that commit first and point the `replace` at it (see
  below), then `go mod tidy`.
- `docs/` in three languages (en, ja, zh): keep both sides' paragraphs.
- `internal/actions/actions.go`, `actions.go`: both sides add actions; keep
  both, in the menus' order.
- `mygo.json`: always upstream's. Its version stays upstream's, so that the
  fork reports the version it is based on.

`go tool mygo generate` may leave `locales/catalog_gen.go` changed in line
endings only (`git diff` shows nothing): `git checkout -- locales/catalog_gen.go`.

## Rules for the fork's changes

- MyGo is forked too: `FallDownTheSystem/mygo`, its branch `kero`, on the
  commit of upstream's MyGo that Kero's `go.mod` requires. `go.mod`
  replaces MyGo with it. A fix or a feature Kero needs of MyGo goes there,
  a commit each, and a bug is reported upstream too. Work on both with a
  `go.work` beside `go.mod` (`use (. ..\mygo)`), which git ignores. Once
  `kero` is pushed, point the `replace` at its new commit:

  ```sh
  GOWORK=off go list -m -json github.com/FallDownTheSystem/mygo@<commit>  # its Version
  go mod edit -replace github.com/egoist/mygo=github.com/FallDownTheSystem/mygo@<Version>
  GOWORK=off go mod tidy
  ```
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

## Building and installing by hand

A release's installer is `kero-setup-X.Y.Z-fork.N-amd64.exe` on its
release page (`Kero.Setup.X.Y.Z-fork.N.amd64.exe` before 2.1.24, whose
MyGo names installers after the app's slug). To build one here instead,
Go is needed (`winget install GoLang.Go`; an older Go than `go.mod`'s
downloads the newer one itself), and `jq`, and nothing else: the build
downloads NSIS and `ghostty-vt.dll`.

Build from the release's tag, with `mygo.json` set as the Fork release
workflow sets it, so that the install reports the fork's version and
updates to the fork's releases, verified with the fork's key. In
PowerShell:

```powershell
git switch --detach v2.1.21-fork.1
$v = (git describe --tags --exact-match HEAD).TrimStart('v')
$key = '5mYJ1/05W+MiC0LUkGGc9lyuCDo600ZL4idnU14+kWI='
jq --arg v $v --arg repo FallDownTheSystem/kero --arg key $key `
        '.version = $v | .updates.github = $repo | .updates.publicKey = $key | .updates.changelog = "FORK-CHANGELOG.md"' `
        mygo.json > "$env:TEMP\mygo.json"
Copy-Item "$env:TEMP\mygo.json" mygo.json -Force
go tool mygo build -platform windows/amd64
git checkout -- mygo.json locales/catalog_gen.go
git switch main
```

leaves `build/windows-amd64/kero-setup-X.Y.Z-fork.N-amd64.exe`.

- `git switch --detach` builds the tag's code, so that the version agrees
  with what is built: on a commit after the tag, `git describe --tags
  --exact-match` fails. From one release to the next, only that line
  changes, as `$v` comes from the tag.
- The key is the workflow's `UPDATES_PUBLIC_KEY`.
- `git checkout` puts `mygo.json` back to upstream's, which is what the
  repository keeps.
- The build warns that it doesn't sign an update: only a release, with the
  secret key, uploads one.

Built from `mygo.json` as it is in the repository, the install reports
upstream's version and checks egoist's releases for updates, and with
**Download and install updates automatically** on (Settings › General),
the next official release replaces it.

Before running the installer:

- Stop kerod, which keeps running after the app quits and holds `Kero.exe`:
  quit with **End All Sessions and Quit** (the tray icon's menu, or the Kero
  menu), or run `kero daemon stop`. Both end the sessions.
- Start the installer from Explorer, or a normal terminal. The installer
  starts Kero when it is done, and Kero and kerod keep the environment it
  had: started from a shell that sets `NO_COLOR` (as an agent's does),
  every pane loses its colors.

The installer is not signed: SmartScreen doesn't ask about a file built on
this machine, but a work machine's security tools may still flag it.
