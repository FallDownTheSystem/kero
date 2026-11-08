package daemon

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"kero/internal/ama"
	"kero/internal/proto"
)

// Ama is the app's helper agent: a session of no project, in a folder of
// its own (ama.Dir), which kerod keeps one of. It drives the app as kero
// do, kero focus and kero browser do, without the Remote Control the
// user gives other agents: kerod gives its panes a secret, $KERO_AMA, that
// those commands pass on, and tells the app a call came with it
// (proto.Message's Ama). The secret is an HMAC of the session's id with a
// key of this kerod's alone, which a restart makes anew, as it starts the
// panes again: no file holds it, and it names Ama's session alone.

// amaEnv is the variable the secret is in.
const amaEnv = "KERO_AMA"

// amaSecret is the key the secrets are made with, of this kerod's run.
func (d *Daemon) amaSecret() []byte {
	d.amaOnce.Do(func() {
		d.amaKey = make([]byte, 32)
		rand.Read(d.amaKey)
	})
	return d.amaKey
}

// amaToken is the secret of Ama's session sid.
func (d *Daemon) amaToken(sid string) string {
	m := hmac.New(sha256.New, d.amaSecret())
	m.Write([]byte(sid))
	return hex.EncodeToString(m.Sum(nil))
}

// markAma marks sid as Ama's session, before its panes start, so that
// they, and those split from them or restarted, get its secret.
func (d *Daemon) markAma(sid string) { d.amaSIDs.Store(sid, true) }

// amaEnvOf is the environment a pane of session sid adds: the secret, for
// Ama's.
func (d *Daemon) amaEnvOf(sid string) []string {
	if _, ok := d.amaSIDs.Load(sid); !ok {
		return nil
	}
	return []string{amaEnv + "=" + d.amaToken(sid)}
}

// fromAma reports whether token is the secret of Ama's session, which is
// open.
func (d *Daemon) fromAma(token string) bool {
	if token == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sessions {
		if s.ama && hmac.Equal([]byte(token), []byte(d.amaToken(s.id))) {
			return true
		}
	}
	return false
}

// prepareAma writes Ama's folder before its session starts in it: its
// instructions, which Kero keeps, and the user's file, which it doesn't.
func (d *Daemon) prepareAma() (string, error) {
	if err := ama.Prepare(); err != nil {
		return "", err
	}
	return ama.Dir(), nil
}

// otherAmas closes the Ama sessions but keep, as a new one replaces them:
// there is one Ama.
func (d *Daemon) otherAmas(keep string) {
	d.mu.Lock()
	var ids []string
	for _, s := range d.sessions {
		if s.ama && s.id != keep {
			ids = append(ids, s.id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.removeSession(id)
	}
}

// createAma starts Ama's session, as p asks, in Ama's folder and of no
// project, at the end of the sessions; the one there was closes once it
// started, as New Chat in the app replaces it.
func (d *Daemon) createAma(p proto.CreateParams) (proto.Session, error) {
	dir, err := d.prepareAma()
	if err != nil {
		return proto.Session{}, err
	}
	s := &session{id: newID(), name: strings.TrimSpace(p.Name), created: time.Now().UnixMilli(), ama: true}
	d.markAma(s.id)
	p.Worktree, p.URL, p.File, p.Preview, p.Review = "", "", "", false, nil
	pn, err := d.startPane(s.id, p, dir)
	if err != nil {
		d.amaSIDs.Delete(s.id)
		return proto.Session{}, err
	}
	s.panes, s.focus = []*pane{pn}, pn.info.ID
	d.mu.Lock()
	d.sessions = append(d.sessions, s)
	d.panes[pn.info.ID] = pn
	d.mu.Unlock()
	d.otherAmas(s.id)
	d.mu.Lock()
	info := d.sessionInfo(s)
	d.mu.Unlock()
	d.changed()
	d.log.Printf("Ama's session %s created in %s", s.id, dir)
	return info, nil
}
