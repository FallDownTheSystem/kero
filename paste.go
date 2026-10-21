package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/transfer"

	"kero/internal/proto"

	"kero/locales"
)

// maxUpload bounds what a paste into a remote pane uploads.
const maxUpload = 64 << 20

// remotePaste takes a paste into a pane of another machine, where this
// machine's paths mean nothing: the files and the image it holds go to
// the host (paste.file), and their paths there are pasted, which the
// programs there read, as Claude Code an image's. Text pastes as it is.
func (a *App) remotePaste(id string, d transfer.Data) bool {
	h := a.hostOf(id)
	if !h.remote() {
		return false
	}
	files, _ := d.Files()
	var image []byte
	if len(files) == 0 {
		if b, err := d.Read(transfer.PNG); err == nil && len(b) > 0 {
			image = b
		}
	}
	if len(files) == 0 && image == nil {
		return false
	}
	cl := h.cl
	if cl == nil {
		a.toast(locales.HostNotConnected(h.name))
		return true
	}
	// What goes up: the image's bytes, or a file, read off the main
	// thread.
	type upload struct {
		name, file string
		data       []byte
	}
	var ups []upload
	total := len(image)
	if image != nil {
		ups = append(ups, upload{name: "pasted-image.png", data: image})
	}
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || st.IsDir() {
			a.toast(locales.PasteNotAFile(filepath.Base(f)))
			return true
		}
		total += int(st.Size())
		if total > maxUpload {
			a.toast(locales.PasteTooLarge(maxUpload >> 20))
			return true
		}
		ups = append(ups, upload{name: filepath.Base(f), file: f})
	}
	if total > 1<<20 {
		a.toast(locales.PasteUploading(h.name))
	}
	go func() {
		var paths []string
		for _, u := range ups {
			data := u.data
			if u.file != "" {
				b, err := os.ReadFile(u.file)
				if err != nil {
					a.update(func() { a.toast(err.Error()) })
					return
				}
				data = b
			}
			var r proto.PasteFile
			if err := cl.Call("paste.file", proto.PasteFileParams{Name: u.name, Data: data}, &r); err != nil {
				a.update(func() { a.toast(fmt.Sprintf("%s: %v", h.name, err)) })
				return
			}
			paths = append(paths, shellEscape(r.Path))
		}
		a.update(func() {
			if e := a.terms[id]; e != nil && e.term != nil {
				e.term.Paste(strings.Join(paths, " "))
			}
		})
	}()
	return true
}

// shellEscape writes a path as one word of a POSIX shell, as dropped
// files' are: a backslash before what the shell would read otherwise.
func shellEscape(p string) string {
	var b strings.Builder
	for _, r := range p {
		safe := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+@%:,=", r) || r > 0x7f
		if !safe {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
