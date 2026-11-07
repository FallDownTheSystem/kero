package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/commitmsg"

	"kero/locales"
)

// branchNameGroup is Settings › Git's worktree branch names: what goes
// before a branch named from a new worktree's task, and how it is named.
func (a *App) branchNameGroup(c *ui.Context, pal *palette) {
	s := &a.set
	settingsGroup(c, pal, locales.SetBranchNames(), locales.SetBranchNamesDetail(), func() {
		settingRow(c, pal, locales.SetBranchPrefix(), "", func() {
			segmented(c, "worktree-branch-prefix", &s.WorktreeBranchPrefix, []string{"semantic", "static", "custom"},
				[]string{locales.SetBranchPrefixSemantic(), locales.SetBranchPrefixStatic(), locales.SetBranchPrefixCustom()})
		})
		if s.WorktreeBranchPrefix == "static" {
			settingRow(c, pal, locales.SetBranchStatic(), locales.SetBranchStaticDetail(), func() {
				if !a.ss.branchStaticEdit {
					a.ss.branchStatic = s.WorktreeBranchStatic
				}
				in := textField(c, "worktree-branch-static", &a.ss.branchStatic, "kero/", 160, pal).Font("monospace")
				a.ss.branchStaticEdit = in.Focused()
				// Kept as it is submitted, or as the field loses the focus;
				// an empty one keeps the prefix it had.
				keep := func() {
					if v := strings.TrimSpace(a.ss.branchStatic); v != "" {
						a.set.WorktreeBranchStatic = v
					}
				}
				in.OnSubmit(keep)
				if !a.ss.branchStaticEdit && strings.TrimSpace(a.ss.branchStatic) != s.WorktreeBranchStatic {
					keep()
				}
			})
		}
		promptSetting(c, pal, "branch-prompt", locales.SetBranchPrompt(), locales.SetBranchPromptDetail(), &s.WorktreeBranchPrompt, commitmsg.DefaultBranchPrompt, &a.ss.branchPrompt)
	})
}
