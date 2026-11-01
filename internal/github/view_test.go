package github

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// A pull request's page, as gh pr view prints it: users' and teams'
// requests, reviews, the conversation, and check runs and commit statuses
// in one list.
func TestParseDetail(t *testing.T) {
	out := []byte(`{"number":7,"title":"Fix","body":"b","url":"u","author":{"login":"ann"},"headRefName":"fix","headRepositoryOwner":{"login":"ann"},
		"isCrossRepository":true,"baseRefName":"main","isDraft":true,"reviewDecision":"REVIEW_REQUIRED","updatedAt":"2026-10-26T20:41:56Z",
		"state":"OPEN","headRefOid":"h1","baseRefOid":"b1","additions":3,"deletions":1,"changedFiles":2,"createdAt":"2026-10-25T10:00:00Z",
		"labels":[{"name":"bug"}],
		"reviewRequests":[{"__typename":"User","login":"bob"},{"__typename":"Team","name":"Core","slug":"core"}],
		"reviews":[{"author":{"login":"cid"},"state":"CHANGES_REQUESTED","body":"no","submittedAt":"2026-10-26T11:00:00Z"}],
		"comments":[{"author":{"login":"dee"},"body":"hi","createdAt":"2026-10-26T12:00:00Z","url":"c"}],
		"statusCheckRollup":[
			{"__typename":"CheckRun","name":"build","workflowName":"CI","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"d1"},
			{"__typename":"CheckRun","name":"test","workflowName":"CI","status":"IN_PROGRESS","conclusion":"","detailsUrl":"d2"},
			{"__typename":"StatusContext","context":"ci/lint","state":"FAILURE","targetUrl":"d3"},
			{"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED"}]}`)
	d, err := parseDetail(out)
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != 7 || !d.Draft || !d.CrossRepo || d.HeadOwner != "ann" || d.State != "OPEN" || d.HeadOID != "h1" || d.ChangedFiles != 2 || d.Checks != "failure" {
		t.Errorf("pull request %+v", d)
	}
	if !slices.Equal(d.Labels, []string{"bug"}) || !slices.Equal(d.Requested, []string{"bob", "core"}) {
		t.Errorf("labels %q, requested %q", d.Labels, d.Requested)
	}
	if len(d.Reviews) != 1 || d.Reviews[0].Author != "cid" || d.Reviews[0].State != "CHANGES_REQUESTED" {
		t.Errorf("reviews %+v", d.Reviews)
	}
	if len(d.Comments) != 1 || d.Comments[0].Author != "dee" || d.Comments[0].Body != "hi" {
		t.Errorf("comments %+v", d.Comments)
	}
	want := []Check{{"build", "CI", "success", "d1"}, {"test", "CI", "pending", "d2"}, {"ci/lint", "", "failure", "d3"}, {"docs", "", "skipped", ""}}
	if !slices.Equal(d.CheckRuns, want) {
		t.Errorf("checks %+v", d.CheckRuns)
	}
}

// Comments on lines come a page after the other, each page an array.
func TestParseLineComments(t *testing.T) {
	out := []byte(`[{"id":1,"in_reply_to_id":null,"path":"a.go","body":"x","created_at":"2026-10-26T20:41:56Z","html_url":"u1","side":"RIGHT","line":12,"start_line":10,"original_line":12,"user":{"login":"ann"}}]
[{"id":2,"in_reply_to_id":1,"path":"a.go","body":"y","created_at":"2026-10-26T21:00:00Z","html_url":"u2","side":"RIGHT","line":null,"start_line":null,"original_line":12,"user":{"login":"bob"}},
 {"id":3,"in_reply_to_id":null,"path":"b.go","body":"z","created_at":"2026-10-26T21:00:00Z","html_url":"u3","side":"RIGHT","line":null,"original_line":null,"subject_type":"file","user":{"login":"cid"}}]`)
	cs, err := parseLineComments(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("comments %+v", cs)
	}
	if c := cs[0]; c.ID != 1 || c.ReplyTo != 0 || c.Line != 12 || c.StartLine != 10 || c.Author != "ann" || c.Side != "RIGHT" {
		t.Errorf("first %+v", c)
	}
	if c := cs[1]; c.ReplyTo != 1 || c.Line != 0 || c.OriginalLine != 12 || c.File {
		t.Errorf("reply %+v", c)
	}
	// On the file: no lines, and not outdated for it.
	if c := cs[2]; !c.File || c.Line != 0 || c.Path != "b.go" {
		t.Errorf("on the file %+v", c)
	}
}

// fakeGH puts a gh on PATH that logs each call, its arguments and its
// input, as a line of log, and answers as script does.
func fakeGH(t *testing.T, script string) (log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gh as a shell script")
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	sh := "#!/bin/sh\nin=$(cat)\necho \"$* <$in>\" >> " + log + "\n" + script
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// A review with a comment on a file goes pending with its comments on
// lines, gets the file's comment, then is submitted with its verdict and
// body; when the file's comment fails, the pending review goes.
func TestPostReviewFileComments(t *testing.T) {
	r := Repo{Host: "github.com", Owner: "me", Name: "demo"}
	v := NewReview{Commit: "h1", Body: "Looks good.", Event: EventApprove, Comments: []NewLineComment{
		{Path: "a.go", Side: "RIGHT", Line: 3, Body: "on a line"},
		{Path: "b.go", File: true, Body: "on the file"},
	}}
	log := fakeGH(t, `case "$*" in
*graphql*) echo '{"data":{}}' ;;
*/events*) echo '{"id":42,"html_url":"https://github.com/me/demo/pull/7#review-42"}' ;;
*) echo '{"id":42,"node_id":"PRR_42","html_url":""}' ;;
esac
`)
	url, err := PostReview(context.Background(), t.TempDir(), r, 7, v)
	if err != nil || url != "https://github.com/me/demo/pull/7#review-42" {
		t.Fatalf("posted %q, %v", url, err)
	}
	b, _ := os.ReadFile(log)
	calls := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(calls) != 3 {
		t.Fatalf("calls %q", calls)
	}
	// Pending: no event, no body, the line's comment alone.
	if c := calls[0]; !strings.Contains(c, "POST repos/me/demo/pulls/7/reviews --input -") || strings.Contains(c, `"event"`) || strings.Contains(c, "Looks good") ||
		!strings.Contains(c, `"path":"a.go"`) || strings.Contains(c, "b.go") {
		t.Errorf("pending review %q", c)
	}
	if c := calls[1]; !strings.Contains(c, "graphql") || !strings.Contains(c, "subjectType: FILE") || !strings.Contains(c, "review=PRR_42") ||
		!strings.Contains(c, "path=b.go") || !strings.Contains(c, "body=on the file") {
		t.Errorf("the file's comment %q", c)
	}
	if c := calls[2]; !strings.Contains(c, "reviews/42/events") || !strings.Contains(c, `"event":"APPROVE"`) || !strings.Contains(c, `"body":"Looks good."`) {
		t.Errorf("submitted %q", c)
	}

	log = fakeGH(t, `case "$*" in
*graphql*) echo 'no such path' >&2; exit 1 ;;
*DELETE*) echo '' ;;
*) echo '{"id":42,"node_id":"PRR_42","html_url":""}' ;;
esac
`)
	if _, err := PostReview(context.Background(), t.TempDir(), r, 7, v); err == nil || !strings.Contains(err.Error(), "b.go") {
		t.Errorf("a failed file comment: %v", err)
	}
	b, _ = os.ReadFile(log)
	if !strings.Contains(string(b), "--method DELETE repos/me/demo/pulls/7/reviews/42") || strings.Contains(string(b), "/events") {
		t.Errorf("after the failure %s", b)
	}

	// With comments on lines alone, it posts as one call, as before.
	log = fakeGH(t, `echo '{"id":43,"html_url":"https://github.com/me/demo/pull/7#review-43"}'`)
	v.Comments = v.Comments[:1]
	if url, err := PostReview(context.Background(), t.TempDir(), r, 7, v); err != nil || url != "https://github.com/me/demo/pull/7#review-43" {
		t.Fatalf("posted %q, %v", url, err)
	}
	if b, _ := os.ReadFile(log); strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), `"event":"APPROVE"`) {
		t.Errorf("calls %s", b)
	}
}

// Threads come by the id of the comment that starts each.
func TestParseThreads(t *testing.T) {
	out := []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[
		{"id":"T1","isResolved":true,"comments":{"nodes":[{"databaseId":11}]}},
		{"id":"T2","isResolved":false,"comments":{"nodes":[{"databaseId":12}]}},
		{"id":"T3","isResolved":false,"comments":{"nodes":[]}}]}}}}}`)
	ts, err := parseThreads(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 2 || ts[11] != (Thread{"T1", true}) || ts[12] != (Thread{"T2", false}) {
		t.Errorf("threads %+v", ts)
	}
}
