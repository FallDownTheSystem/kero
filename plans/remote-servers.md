# Plan: Kero on remote servers

Status: future plan, not started. Written 2026-10-20 after looking at Rex
(superlogical.com/rex), which ships a Linux server mode, multi-user servers and
remote access over Tailscale.

## Goal

Run sessions on another machine, such as a Linux dev box, a cloud VM or a
beefy desktop, and see them in the same Kero window as local ones: the same
sidebar, statuses, notifications, `kero` CLI and agent skill. Quitting Kero or
closing the laptop leaves remote agents running, as it does locally.

Not goals, for now: sharing one session between two people, a web client, an
iOS client.

## Where we start

kerod is already most of a server:

- It owns the sessions and runs them on their own. Quitting the app leaves them
  running.
- Each pane has its own headless libghostty-vt emulator (`internal/daemon/session.go`).
  An attach stream carries rendered diffs from `terminal.Mirror`, not raw PTY
  bytes. A client that falls behind is dropped and reattaches from a fresh
  frame. Both suit slow or lossy links better than forwarding a PTY.
- Control is newline-delimited JSON (`Hello`, then `{id, method, params}`), and
  attach uses binary frames (`internal/proto/frame.go`). Neither cares what
  carries it.
- `pty_linux.go` and `proc_linux.go` exist, and libghostty-vt ships for linux
  amd64 and arm64.

What ties it to one machine today:

- **Transport:** a Unix socket only (`proto.SocketPath`), mode 0600.
  - There's no authentication and no peer-credential check. The 0600 chmod
    comes after `listen`, so there's a short window before it applies.
- **Lifetime:** kerod exits 30s after its last session and connection close,
  and the app starts it on demand (`client.Connect` runs `kero __kerod`).
- **Packaging:** there's one binary. `main.go` imports the UI, and the daemon
  imports `mygo` only for `mygo.App.Version()`. Only a darwin-arm64 build
  exists.
- **The app assumes local paths:**
  - The Open Project dialog picks a local folder.
  - ⌘-clicking a file path runs `os.Stat` and opens the file locally.
  - Image paste gives programs local file paths.
  - `ShortDir` and display names compare against the local home.
- **Shell lookup:** `internal/shell/account_unix.go` reads `/etc/passwd`
  directly, so LDAP and SSSD accounts are missed.

## Phase 1: headless kerod on Linux

The smallest useful step: kerod running as a service on a Linux box, driven by
the `kero` CLI there. That already helps agents on that box, before any remote
UI exists.

1. **A daemon-only build** (`cmd/kerod`, or a build tag) without the UI
   packages.
   - Pass the version in with `-ldflags` instead of `mygo.App.Version()`.
   - Check that it starts on a machine with no display.
2. **`kero daemon serve`:** run in the foreground, with no idle exit, logging
   to stderr. This is the entry point for systemd.
3. **`kero daemon install-service`:** write and enable a systemd *user* unit
   (`~/.config/systemd/user/kerod.service`), plus `loginctl enable-linger` so
   it survives logout.
   - Remove the service with `kero daemon uninstall-service`.
4. **Release artifacts:** a linux amd64/arm64 tarball with `kero`, kerod and
   `libghostty-vt.so`, built in CI from the same tags as the Mac build. Debian
   or Arch packages and a Nix flake come later, on demand.
5. **Fixes that matter once a box has several users:**
   - Set the umask before `listen` instead of chmodding after.
   - Check the peer's uid with `SO_PEERCRED` and reject other users.
   - Look up the shell through NSS (`getent passwd`) instead of parsing
     `/etc/passwd`.

## Phase 2: remote access, one user

The app connects to a kerod on another machine that runs as the same person.

### Transport: SSH first

The app runs `ssh <host> kero daemon bridge`. That command relays stdio to the
remote kerod's Unix socket, starting kerod if it isn't running. Each Kero
connection (one control connection, one attach per visible pane) becomes one
`ssh` channel. With `ControlMaster`, or one `ssh` process multiplexing
channels, it shares one TCP connection.

- **For:** no new authentication, ports or certificates. It uses the user's
  keys, agent, `~/.ssh/config`, jump hosts and `known_hosts`. It works with
  every host the user can already reach, including Tailscale SSH.
- **Against:** needs `ssh` on the client (fine on macOS, Linux and Windows 10+),
  plus one more process.

A direct network listener (Phase 4) comes after. SSH covers most real setups,
and it moves the security model onto something already trusted.

### Protocol changes

- `Hello` negotiates **compression** for attach frames (zstd, or deflate from
  the standard library). Rendered diffs compress well, and they cost bandwidth
  only on scroll-heavy output.
- **Version skew:** a remote kerod of another version can't be restarted
  quietly, because it may hold someone's long-running agents. Show it
  per host, with "Update Kero on <host>", which runs the installer over the
  same SSH link. Keep the no-compat rule (Kero is unreleased),
  but the UI has to explain the mismatch rather than fail.
- **Heartbeats** on the control connection, so a dead link shows as
  "reconnecting" within seconds, and the app reattaches when it comes back.

### App changes

- **Hosts:** Settings › Hosts lists `ssh` destinations, and `host = name=ssh-destination` in the
  config file. The sidebar groups projects by host, with the local machine
  first and unlabeled. Each host has its own connection state.
- **Open Project on a host:** a folder picker served by a new `fs.list` RPC.
  The NSOpenPanel stays for the local machine.
- **⌘-click a path in a remote pane:** resolve it on the host (`fs.stat`).
  Open it there in `$EDITOR` in a new pane, or copy the path. Never open a
  local file with the same name.
- **Image paste into a remote pane:** upload the image to a temp dir on the
  host (`paste.file`), then paste the *remote* path, so TUIs that take image
  paths keep working.
- **Settings split:**
  - Appearance and keyboard come from the app.
  - Shell, `env`, scrollback and `restore-sessions` come from the host's own
    config, since kerod reads them there.
  - Settings shows which is which, and `kero config` edits the host's.

### CLI and agents

- `kero -H <host> ls` and `KERO_HOST` drive a remote kerod over the same
  bridge. `kero hosts` lists them.
- Inside a remote pane, `kero` already talks to the remote kerod through
  `KERO_SOCKET`, so the skill works there unchanged. `kero skill setup` needs
  running on the host.
- **Remote Control from a remote agent:** `kero focus` and `kero do` reach the
  local app, which is a new kind of trust (code on a server driving my
  laptop's UI). Add a per-host toggle, off by default for remote hosts.

## Phase 3: multi-user servers

Rex has three server modes:

- **Personal:** one kerod per user.
- **Shared:** one service account that a group connects to.
- **Login:** many OS users, with one front door that routes each to their own
  account.

For Kero:

- **Personal is already multi-user-safe once Phase 1's fixes land.** Each user
  has their own kerod, `~/.config/kero` at 0700, a 0600 socket and a uid
  check. With the SSH transport, each person connects as themselves to their
  own kerod. That *is* the login-server mode, with sshd as the front door and
  PAM done by sshd. No kero code runs as root.
- **Login server without SSH** only matters for a network listener (Phase 4).
  A root-owned `kero gateway` (a systemd system service) would:
  - authenticate the connection, by Tailscale identity or a token;
  - map it to an OS account through an accounts file;
  - start that user's kerod with `systemd-run --uid=<user> --user` or through
    PAM;
  - relay the connection to that user's socket.

  It's the only piece that runs privileged, so keep it small and audit it.
- **Shared server** (several people's programs running as one `kero` account)
  is the weakest model: everyone can read everyone's sessions. Skip it unless
  someone asks. A shared session for pairing is a different feature, with its
  own design.

## Phase 4: network listener and Tailscale

For machines without SSH, or for the iOS client later.

- **`listen = tailscale` with tsnet built into kerod (or the gateway):**
  - The server joins the tailnet as its own node.
  - Every connection is identified with `WhoIs`.
  - Access defaults to the tailnet user who owns the machine, with an
    allowlist for more.
  - No ports to open, and TLS comes from WireGuard.
  - Cost: binary size, and a node per server.
- **The alternative, `tailscale serve --tcp`, in front of a local TCP listener:**
  no tsnet dependency, but the identity has to come from Tailscale's headers or
  `whois` on the local API. It's worth a spike to compare.
- **Plain TCP and TLS with a per-host token** for everything else:
  - The token is created by `kero daemon token`.
  - Self-signed certificates are pinned on first connect, as SSH host keys are.
  - Plain unencrypted TCP is never offered.
- Listening is off by default. Out of the box kerod listens only on its Unix
  socket.

## Security notes

- Everything a remote pane sends is untrusted, as it is locally. OSC 52
  clipboard reads keep their per-pane `ask` policy, and OSC 7501 text is still
  neutralized.
  - A remote host must not be able to pose as a local pane: show the host's
    name on its notifications and in the sidebar.
- Remote Control is per host and off by default for remote hosts (above).
- The bridge and gateway authenticate before they parse any `Hello` past its
  version.

## Order and rough size

1. **Phase 1, headless Linux kerod:** small to medium. Mostly packaging and
   CI, plus the socket and uid fixes. Worth doing on its own.
2. **Phase 2, SSH remote:** large. Hosts in the app, `bridge`, compression,
   reconnects, `fs.*` and `paste.file` RPCs, and the settings split.
3. **Phase 3:** mostly free with SSH. The gateway waits for Phase 4.
4. **Phase 4, Tailscale and network listener:** medium, after a tsnet versus
   `serve` spike.

## Open questions

- Does `kero __kerod` start on a Linux machine with no display, given that the
  binary links the UI? This decides whether Phase 1 needs a separate build at
  all.
- How does input latency feel over a slow link? kerod renders every frame, so
  a typed key needs a round trip before it shows. Measure before considering
  local echo prediction.
- Windows hosts: kerod already runs on Windows with ConPTY. Is SSH to a
  Windows box common enough to support in Phase 2?
- Should a remote session show the host's own terminal theme, or always the
  app's? Today the app styles the terminal, so it would be the app's.
