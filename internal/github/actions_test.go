package github

import (
	"slices"
	"testing"
)

// What a pull request's actions depend on, as GitHub's GraphQL says it:
// the methods the repository allows in GitHub's order, its auto-merge's
// method lowercased as gh takes it, and a branch deleted since.
func TestParseActions(t *testing.T) {
	out := []byte(`{"data":{"repository":{"mergeCommitAllowed":false,"squashMergeAllowed":true,"rebaseMergeAllowed":true,
		"autoMergeAllowed":true,"deleteBranchOnMerge":false,"viewerPermission":"TRIAGE",
		"pullRequest":{"id":"PR_1","state":"MERGED","isDraft":false,"headRefName":"fix/a","headRefOid":"h1","baseRefName":"main",
		"isCrossRepository":true,"mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","viewerCanUpdate":true,
		"viewerCanUpdateBranch":false,"viewerCanDeleteHeadRef":true,"autoMergeRequest":{"mergeMethod":"SQUASH"},
		"headRef":null,"headRepository":{"nameWithOwner":"ann/demo"}}}}}`)
	a, err := parseActions(out)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Methods, []string{MethodSquash, MethodRebase}) {
		t.Errorf("methods %q", a.Methods)
	}
	if a.CanWrite || !a.CanUpdate || !a.CanDeleteBranch || a.CanUpdateBranch {
		t.Errorf("access %+v", a)
	}
	if a.AutoMerge != MethodSquash || a.HeadExists || a.HeadRepo != "ann/demo" || a.ID != "PR_1" || a.State != "MERGED" {
		t.Errorf("pull request %+v", a)
	}
	if _, err := parseActions([]byte(`{"data":{"repository":{"pullRequest":null}}}`)); err == nil {
		t.Error("no pull request parsed")
	}
}

func TestRefPath(t *testing.T) {
	if got := refPath("feat/a#b c"); got != "feat/a%23b%20c" {
		t.Errorf("refPath %q", got)
	}
}
