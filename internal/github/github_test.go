package github

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/egoist/kero":                 "egoist/kero",
		"https://github.com/egoist/kero.git":             "egoist/kero",
		"https://user@github.com/egoist/kero.git/":       "egoist/kero",
		"git@github.com:FallDownTheSystem/kero.git":      "FallDownTheSystem/kero",
		"ssh://git@github.com/egoist/kero.git":           "egoist/kero",
		"ssh://git@ssh.github.com:443/egoist/kero.git":   "egoist/kero",
		"https://github.example.com/team/app.git":        "github.example.com/team/app",
		"https://gitlab.com/team/app.git":                "",
		"/srv/git/app.git":                               "",
		`C:\src\app`:                                     "",
		"C:/src/app":                                     "",
		"https://github.com/egoist":                      "",
		"https://github.com/egoist/kero/tree/main/extra": "",
	} {
		r, ok := ParseRemote(raw)
		got := ""
		if ok {
			got = r.Slug()
		}
		if got != want {
			t.Errorf("ParseRemote(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A pull request's checks sum up to their worst: a failure over one still
// running, one running over successes.
func TestChecks(t *testing.T) {
	ok := ghCheck{Status: "COMPLETED", Conclusion: "SUCCESS"}
	skipped := ghCheck{Status: "COMPLETED", Conclusion: "SKIPPED"}
	running := ghCheck{Status: "IN_PROGRESS"}
	failed := ghCheck{Status: "COMPLETED", Conclusion: "FAILURE"}
	status := ghCheck{State: "SUCCESS"}
	statusPending := ghCheck{State: "PENDING"}
	for _, c := range []struct {
		cs   []ghCheck
		want string
	}{
		{nil, ""},
		{[]ghCheck{ok, skipped, status}, "success"},
		{[]ghCheck{ok, running}, "pending"},
		{[]ghCheck{ok, statusPending}, "pending"},
		{[]ghCheck{running, failed}, "failure"},
	} {
		if got := checks(c.cs); got != c.want {
			t.Errorf("checks(%+v) = %q, want %q", c.cs, got, c.want)
		}
	}
}

// The compare page of a fork's branch names its owner, and carries the
// title and the body, escaped.
func TestCompareURL(t *testing.T) {
	r := Repo{Host: "github.com", Owner: "egoist", Name: "kero"}
	n := New{Base: "main", Head: "fix/a b", HeadOwner: "FallDownTheSystem", Title: "Fix & test", Body: "Line 1\nLine 2"}
	got := CompareURL(r, n)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://github.com/egoist/kero/compare/main...FallDownTheSystem:fix%2Fa%20b?") {
		t.Errorf("CompareURL = %s", got)
	}
	q := u.Query()
	if q.Get("expand") != "1" || q.Get("title") != n.Title || q.Get("body") != n.Body {
		t.Errorf("query %v", q)
	}
	// A branch of the repository's own owner needs no owner.
	n.HeadOwner = "egoist"
	if h := n.head(r); h != "fix/a b" {
		t.Errorf("head = %q", h)
	}
}
