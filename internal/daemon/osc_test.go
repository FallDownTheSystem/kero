package daemon

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestScannerAcrossReads(t *testing.T) {
	msg := base64.StdEncoding.EncodeToString([]byte("Installing updates"))
	stream := "hello \x1b]0;title\x07 \x1b]7501;state=working:app=brew:msg=" + msg + "\x1b\\ world \x1b]133;A\x07 \x1b]9;4;1;40\x07"
	var got []string
	var o oscScanner
	// Byte at a time: sequences split anywhere still come whole.
	for i := range len(stream) {
		o.scan([]byte{stream[i]}, func(b []byte) { got = append(got, string(b)) })
	}
	want := []string{"7501;state=working:app=brew:msg=" + msg, "133;A", "9;4;1;40"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseStatus(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	r, ok := parseStatus("state=blocked:kind=permission:app=terraform:msg=" + b64("Apply 3 to add?") + ":progress=10")
	if !ok || r.rec.State != "blocked" || r.rec.Kind != "permission" || r.rec.App != "terraform" || r.rec.Msg != "Apply 3 to add?" || r.rec.Progress != 10 {
		t.Fatalf("blocked report: %+v %v", r.rec, ok)
	}
	if r, ok := parseStatus("?"); !ok || !r.query {
		t.Fatal("query not recognized")
	}
	for _, bad := range []string{
		"kind=question",                                 // no state
		"state=sleeping",                                // unknown state
		"state=done:msg=" + b64("a\nb"),                 // a control character
		"state=done:msg=!!!",                            // malformed pair, then no msg: fine…
		"state=working:id=a/" + strings.Repeat("x", 40), // segment too long
	} {
		_, ok := parseStatus(bad)
		if bad == "state=done:msg=!!!" {
			if !ok {
				t.Errorf("%q: a malformed pair is skipped, the report kept", bad)
			}
			continue
		}
		if ok {
			t.Errorf("%q accepted", bad)
		}
	}
	// Progress only means something while working or blocked.
	if r, _ := parseStatus("state=done:progress=50"); r.rec.Progress != -1 {
		t.Errorf("progress on done: %d", r.rec.Progress)
	}
	// Bidi overrides are dropped from messages shown outside the grid.
	if r, _ := parseStatus("state=done:msg=" + b64("ok‮gnp.exe")); r.rec.Msg != "okgnp.exe" {
		t.Errorf("bidi not neutralized: %q", r.rec.Msg)
	}
}
