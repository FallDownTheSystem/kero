package github

import (
	"slices"
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
[{"id":2,"in_reply_to_id":1,"path":"a.go","body":"y","created_at":"2026-10-26T21:00:00Z","html_url":"u2","side":"RIGHT","line":null,"start_line":null,"original_line":12,"user":{"login":"bob"}}]`)
	cs, err := parseLineComments(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("comments %+v", cs)
	}
	if c := cs[0]; c.ID != 1 || c.ReplyTo != 0 || c.Line != 12 || c.StartLine != 10 || c.Author != "ann" || c.Side != "RIGHT" {
		t.Errorf("first %+v", c)
	}
	if c := cs[1]; c.ReplyTo != 1 || c.Line != 0 || c.OriginalLine != 12 {
		t.Errorf("reply %+v", c)
	}
}
