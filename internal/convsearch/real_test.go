package convsearch

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kero/internal/proto"
)

// TestRealHistories indexes this machine's own histories, to see how
// long it takes and what it finds: KERO_SEARCH_REAL=<query> runs it.
func TestRealHistories(t *testing.T) {
	q := os.Getenv("KERO_SEARCH_REAL")
	if q == "" {
		t.Skip("KERO_SEARCH_REAL is not set")
	}
	cache := filepath.Join(t.TempDir(), "search.gob")
	for _, round := range []string{"cold", "cached"} {
		ix := New(cache, Sources(proto.Home()))
		start := time.Now()
		ix.Search(proto.SearchParams{Query: q}, time.Now())
		ix.Wait()
		took := time.Since(start)
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		start = time.Now()
		res := ix.Search(proto.SearchParams{Query: q, Limit: 10, Preview: true}, time.Now())
		st, _ := os.Stat(cache)
		size := int64(0)
		if st != nil {
			size = st.Size()
		}
		t.Logf("%s: indexed %d in %v, heap %d MB, cache %d MB; search %v, %d matched", round, res.Indexed, took, ms.HeapAlloc>>20, size>>20, time.Since(start), res.Matched)
		if round == "cached" {
			for _, h := range res.Hits {
				t.Logf("%s %s %s %d turns: %s", h.Agent, time.UnixMilli(h.Updated).Format(time.DateTime), h.Dir, h.Turns, h.Title)
				for _, s := range h.Snippets {
					t.Logf("    user=%v %q %v", s.User, s.Text, s.Marks)
				}
			}
		}
	}
}
