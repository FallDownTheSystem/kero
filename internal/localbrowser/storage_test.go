package localbrowser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// seedPage keeps a sign-in in the site's storage as apps do, with values
// JSON would lose, and a key of the browser's own, which a page can't
// copy, then titles itself seeded.
const seedPage = `<!doctype html><title>x</title><script>
if (location.search === "?seed") (async () => {
	localStorage.setItem("token", "abc.def");
	localStorage.setItem("ünï", "çödé");
	const key = await crypto.subtle.generateKey({name: "HMAC", hash: "SHA-256"}, false, ["sign"]);
	const r = indexedDB.open("auth", 3);
	r.onupgradeneeded = () => {
		const s = r.result.createObjectStore("tokens", {keyPath: "id"});
		s.createIndex("tags", "tags", {multiEntry: true});
		r.result.createObjectStore("blobs", {autoIncrement: true});
		r.result.createObjectStore("keys");
	};
	r.onsuccess = () => {
		const tx = r.result.transaction(["tokens", "blobs", "keys"], "readwrite");
		tx.objectStore("tokens").put({id: "a", tags: ["x", "y"], when: new Date(1700000000000),
			raw: new Uint8Array([1, 2, 255]), m: new Map([["k", new Set([1])]]), big: 12345678901234567890n, nz: -0, nan: NaN});
		tx.objectStore("blobs").put(new Blob(["hello"], {type: "text/plain"}));
		tx.objectStore("keys").put(key, "k");
		tx.objectStore("keys").put("plain", [new Date(5), "x"]);
		tx.oncomplete = () => { r.result.close(); document.title = "seeded"; };
	};
})();
</script>`

// chrome starts the Chromium of $KERO_TEST_CHROME on a profile of its own,
// as a user's browser with remote debugging on, at url, and connects.
func chrome(t *testing.T, url string) (string, *cdp) {
	t.Helper()
	bin := os.Getenv("KERO_TEST_CHROME")
	if bin == "" {
		t.Skip("set KERO_TEST_CHROME to a Chrome or Chromium to read site storage of")
	}
	dir := t.TempDir()
	cmd := exec.Command(bin, "--headless=new", "--no-sandbox", "--no-first-run", "--remote-debugging-port=0", "--user-data-dir="+dir, url)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if u, err := endpoint(dir); err == nil {
			ws, _, err := websocket.Dial(ctx, u, nil)
			if err != nil {
				t.Fatal(err)
			}
			ws.SetReadLimit(256 << 20)
			t.Cleanup(func() { ws.CloseNow() })
			return dir, &cdp{ws: ws}
		}
		select {
		case <-ctx.Done():
			t.Fatal("the browser wrote no DevToolsActivePort")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// tabTitled waits for a tab of the title, and returns its id.
func tabTitled(t *testing.T, c *cdp, title string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for {
		var res struct {
			TargetInfos []struct{ TargetID, Type, Title string }
		}
		if err := c.call(ctx, "", "Target.getTargets", nil, &res); err != nil {
			t.Fatal(err)
		}
		for _, ti := range res.TargetInfos {
			if ti.Type == "page" && ti.Title == title {
				return ti.TargetID
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no tab titled %q", title)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// A site's storage goes from one browser to another as it was: read in a
// tab of the site, written in a page of its origin elsewhere, it reads the
// same there, with its dates, bytes, maps and blobs; a database of the
// site's name the other had is replaced; the browser's own key, which no
// page can copy, is left out and told of.
func TestSiteStorageRoundTrip(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(seedPage))
	}))
	defer site.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	dirA, a := chrome(t, site.URL+"/?seed")
	tabTitled(t, a, "seeded")
	imp, err := Read(ctx, "chrome", dirA, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Sites) != 1 || imp.Sites[0].Origin != site.URL || len(imp.Sites[0].LocalStorage) != 2 {
		t.Fatalf("sites %+v, notes %q", imp.Sites, imp.Notes)
	}
	if len(imp.Notes) != 1 || !strings.Contains(imp.Notes[0], "left out 1 records") {
		t.Errorf("notes %q, want the browser's key left out", imp.Notes)
	}

	// The other browser has a database of the name already, of an older
	// version, which the import replaces.
	_, b := chrome(t, site.URL+"/")
	id := tabTitled(t, b, "x")
	var att struct{ SessionID string }
	if err := b.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": id, "flatten": true}, &att); err != nil {
		t.Fatal(err)
	}
	eval := func(expr string) string {
		t.Helper()
		var r struct {
			Result           struct{ Value string }
			ExceptionDetails json.RawMessage
		}
		if err := b.call(ctx, att.SessionID, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, &r); err != nil {
			t.Fatal(err)
		}
		if r.ExceptionDetails != nil {
			t.Fatalf("%s", r.ExceptionDetails)
		}
		return r.Result.Value
	}
	eval(`new Promise(ok => { const r = indexedDB.open("auth", 1); r.onupgradeneeded = () => r.result.createObjectStore("old"); r.onsuccess = () => { r.result.close(); ok(""); }; })`)
	script, err := WriteScript(imp.Sites[0])
	if err != nil {
		t.Fatal(err)
	}
	var wrote Wrote
	if err := json.Unmarshal([]byte(eval(script)), &wrote); err != nil {
		t.Fatal(err)
	}
	if wrote.Items != 2 || wrote.Databases != 1 || wrote.Records != 3 || len(wrote.Notes) != 0 {
		t.Errorf("wrote %+v", wrote)
	}
	got := eval(`new Promise(ok => { const r = indexedDB.open("auth"); r.onsuccess = () => {
		const db = r.result, tx = db.transaction(["tokens", "blobs", "keys"]);
		const a = tx.objectStore("tokens").get("a"), bl = tx.objectStore("blobs").get(1);
		const ix = tx.objectStore("tokens").index("tags").getAll("y"), k = tx.objectStore("keys").get([new Date(5), "x"]);
		tx.oncomplete = async () => { const v = a.result; ok([db.version, [...db.objectStoreNames].join(","),
			v.when.getTime(), v.raw.join("."), v.m.get("k").has(1), typeof v.big, Object.is(v.nz, -0), Number.isNaN(v.nan),
			await bl.result.text(), bl.result.type, ix.result.length, k.result, localStorage.getItem("ünï")].join(" ")); };
	}; })`)
	if want := "3 blobs,keys,tokens 1700000000000 1.2.255 true bigint true true hello text/plain 1 plain çödé"; got != want {
		t.Errorf("the other browser has\n%s\nwant\n%s", got, want)
	}
}
