// Package convsearch finds the coding agents' conversations by what was
// said in them, as fast-resume does: it reads every conversation the
// agents keep on this machine, whether Kero ran it or not, into an index
// of their messages, the user's and the agents' own, which it keeps on
// disk and reads anew only where a history changed.
package convsearch

import (
	"bytes"
	"encoding/gob"
	"os"
	"runtime"
	"slices"
	"sync"
	"time"
)

// Doc is a conversation as read from an agent's history. Text is its
// messages, each sep, then roleUser or roleAgent, then what was said.
type Doc struct {
	Agent, ID, Dir, Title string
	Started, Updated      int64 // unix ms
	Turns                 int   // the messages the user wrote
	Text                  string
}

const (
	sep       = '\x1e'
	roleUser  = 'u'
	roleAgent = 'a'
)

// Source is where an agent keeps its conversations: List names its
// histories, a file each, and Parse reads one's conversations.
type Source struct {
	Agent string
	List  func() ([]string, error)
	Parse func(path string) ([]Doc, error)
}

// cacheVersion changes as what the index keeps does, or what a Source
// reads: an index of another version is read anew.
const cacheVersion = 1

// refreshEvery is how long a scan of the histories stands before a
// search asks for another.
const refreshEvery = 10 * time.Second

// Index is the conversations of its Sources, by the file each came from.
type Index struct {
	cache   string
	sources []Source

	mu       sync.Mutex
	files    map[string]*entry
	docs     []*doc // of files, the newest of each conversation's
	stale    bool   // docs wants building again
	loaded   bool
	scanning bool
	scanned  time.Time
	done     int
	todo     int
	dirty    bool
	// wake closes as a scan ends, for Wait.
	wake chan struct{}
}

type entry struct {
	Agent     string
	Size, Mod int64
	Docs      []Doc
}

// doc is a Doc with its title, directory and text in lower case, for
// the words a query asks for (Fold).
type doc struct {
	*Doc
	title, dir, text string
}

type cacheFile struct {
	Version int
	Files   map[string]*entry
}

// New makes an index of sources, kept in the file cache.
func New(cache string, sources []Source) *Index {
	return &Index{cache: cache, sources: sources, files: map[string]*entry{}, wake: make(chan struct{})}
}

// kick starts a scan, unless one runs or ran a moment ago.
func (ix *Index) kick(now time.Time) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.scanning || now.Sub(ix.scanned) < refreshEvery {
		return
	}
	ix.scanning = true
	go ix.scan()
}

// Wait waits for the scan that runs, if one does, to end.
func (ix *Index) Wait() {
	ix.mu.Lock()
	scanning, wake := ix.scanning, ix.wake
	ix.mu.Unlock()
	if scanning {
		<-wake
	}
}

func (ix *Index) scan() {
	ix.mu.Lock()
	loaded := ix.loaded
	ix.loaded = true
	ix.mu.Unlock()
	if !loaded {
		ix.load()
	}
	type job struct {
		path string
		src  *Source
		size int64
		mod  int64
	}
	var jobs []job
	seen := map[string]bool{}
	// A source that could not list its histories keeps what it had, as a
	// folder briefly away is no reason to forget its conversations.
	listed := map[string]bool{}
	for i := range ix.sources {
		src := &ix.sources[i]
		paths, err := src.List()
		if err != nil {
			continue
		}
		listed[src.Agent] = true
		for _, p := range paths {
			st, err := os.Stat(p)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			seen[p] = true
			size, mod := st.Size(), st.ModTime().UnixNano()
			ix.mu.Lock()
			e := ix.files[p]
			ix.mu.Unlock()
			if e != nil && e.Size == size && e.Mod == mod && e.Agent == src.Agent {
				continue
			}
			jobs = append(jobs, job{p, src, size, mod})
		}
	}
	ix.mu.Lock()
	for p, e := range ix.files {
		if !seen[p] && listed[e.Agent] {
			delete(ix.files, p)
			ix.stale, ix.dirty = true, true
		}
	}
	ix.done, ix.todo = 0, len(jobs)
	ix.mu.Unlock()
	// The newest first, as the conversations most likely looked for.
	slices.SortFunc(jobs, func(x, y job) int {
		switch {
		case x.mod > y.mod:
			return -1
		case x.mod < y.mod:
			return 1
		}
		return 0
	})
	ch := make(chan job)
	var wg sync.WaitGroup
	for range max(runtime.GOMAXPROCS(0)-1, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				docs, err := j.src.Parse(j.path)
				ix.mu.Lock()
				ix.done++
				// A history that can't be read now, as one being written,
				// keeps what was read of it before.
				if err == nil {
					ix.files[j.path] = &entry{Agent: j.src.Agent, Size: j.size, Mod: j.mod, Docs: docs}
					ix.stale, ix.dirty = true, true
				}
				ix.mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	ix.mu.Lock()
	dirty := ix.dirty
	ix.dirty = false
	ix.mu.Unlock()
	if dirty {
		ix.save()
	}
	ix.mu.Lock()
	ix.scanning, ix.scanned = false, time.Now()
	ix.done, ix.todo = 0, 0
	close(ix.wake)
	ix.wake = make(chan struct{})
	ix.mu.Unlock()
}

// load reads the index as the last scan left it.
func (ix *Index) load() {
	data, err := os.ReadFile(ix.cache)
	if err != nil {
		return
	}
	var cf cacheFile
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&cf) != nil || cf.Version != cacheVersion || cf.Files == nil {
		return
	}
	ix.mu.Lock()
	ix.files, ix.stale = cf.Files, true
	ix.mu.Unlock()
}

func (ix *Index) save() {
	ix.mu.Lock()
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(cacheFile{Version: cacheVersion, Files: ix.files})
	ix.mu.Unlock()
	if err != nil {
		return
	}
	tmp := ix.cache + ".tmp"
	if os.WriteFile(tmp, buf.Bytes(), 0o600) != nil {
		return
	}
	if os.Rename(tmp, ix.cache) != nil {
		os.Remove(tmp)
	}
}

// snapshot is the conversations as now indexed, the newest of each, and
// how the scan stands; ix.mu is held.
func (ix *Index) snapshotLocked() []*doc {
	if !ix.stale {
		return ix.docs
	}
	ix.stale = false
	byID := map[[2]string]*doc{}
	old := map[*Doc]*doc{}
	for _, d := range ix.docs {
		old[d.Doc] = d
	}
	for _, e := range ix.files {
		for i := range e.Docs {
			d := &e.Docs[i]
			k := [2]string{d.Agent, d.ID}
			if o := byID[k]; o != nil && o.Updated >= d.Updated {
				continue
			}
			x := old[d]
			if x == nil {
				x = &doc{Doc: d, title: Fold(d.Title), dir: Fold(d.Dir), text: Fold(d.Text)}
			}
			byID[k] = x
		}
	}
	docs := make([]*doc, 0, len(byID))
	for _, d := range byID {
		docs = append(docs, d)
	}
	slices.SortFunc(docs, func(x, y *doc) int {
		switch {
		case x.Updated > y.Updated:
			return -1
		case x.Updated < y.Updated:
			return 1
		}
		return 0
	})
	ix.docs = docs
	return docs
}

// Find returns the conversation of an agent's id, as last indexed.
func (ix *Index) Find(agent, id string) (Doc, bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	for _, d := range ix.snapshotLocked() {
		if d.Agent == agent && d.ID == id {
			return *d.Doc, true
		}
	}
	return Doc{}, false
}
