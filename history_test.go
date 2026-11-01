package main

import (
	"reflect"
	"testing"
)

// A remote's branch of a local branch's name goes into the local
// branch's chip, before it or after it in the list; one of another name,
// or a tag of the name, keeps its own.
func TestChipRefs(t *testing.T) {
	remotes := []string{"origin", "upstream"}
	for _, tc := range []struct {
		refs string
		want []gitRef
	}{
		{"HEAD -> main, origin/main, upstream/main, tag: v1", []gitRef{
			{name: "main", head: true, tracked: []string{"origin/main", "upstream/main"}},
			{name: "v1", tag: true},
		}},
		{"origin/feat/x, feat/x, origin/other", []gitRef{
			{name: "feat/x", tracked: []string{"origin/feat/x"}},
			{name: "origin/other", remote: true},
		}},
		{"tag: main, origin/main", []gitRef{
			{name: "main", tag: true},
			{name: "origin/main", remote: true},
		}},
	} {
		if got := chipRefs(parseRefs(tc.refs, remotes)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: %+v, want %+v", tc.refs, got, tc.want)
		}
	}
}
