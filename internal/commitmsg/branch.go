package commitmsg

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// A new worktree's branch named from the task its session starts with, by
// the same command that writes commit messages, as t3code names its
// worktrees' branches from the first message.

// DefaultBranchPrompt is how a branch's name is written, unless the user
// says otherwise (settings' worktree-branch-prompt).
const DefaultBranchPrompt = "Two to five lowercase words joined by hyphens that say what the work does, as add-dark-mode or fix-login-redirect."

// TypePlaceholder in a branch's prefix (settings' worktree-branch-prefix) is
// the kind of the work, which the model picks: feat, fix, and the like.
const TypePlaceholder = "{type}"

// DefaultBranchPrefix is the prefix of a named branch unless the user
// says otherwise: the kind of the work, as feat/ or fix/.
const DefaultBranchPrefix = TypePlaceholder + "/"

// branchTypes are the kinds of work a {type} prefix is one of.
var branchTypes = []string{"feat", "fix", "refactor", "docs", "test", "perf", "chore"}

const (
	branchOpen  = "<branch-name>"
	branchClose = "</branch-name>"
)

// BranchPrompt is what a branch's name is asked for with: how to write it
// (instructions, DefaultBranchPrompt when ""), what the prefix asks of the
// reply, the repository's recent branches, as the style to keep, and the
// task. A prefix with TypePlaceholder asks for the kind of the work before
// the name; another, the name alone, which Kero puts the prefix before;
// none leaves the whole of it to the instructions.
func BranchPrompt(instructions, prefix, task string, recent []string) string {
	var b strings.Builder
	b.WriteString("Name the git branch of the task below, which a coding agent is about to work on.\n\n")
	b.WriteString(orDefault(instructions, DefaultBranchPrompt) + "\n\n")
	switch prefix = strings.TrimSpace(prefix); {
	case strings.Contains(prefix, TypePlaceholder):
		b.WriteString("Begin the name with the kind of the work, one of " + strings.Join(branchTypes, ", ") + ", and a slash, as feat/add-dark-mode.\n\n")
	case prefix != "":
		b.WriteString("Kero puts " + prefix + " before the name: reply without a prefix of your own.\n\n")
	}
	b.WriteString("Reply with the name alone, between a line " + branchOpen + " and a line " + branchClose + ". No commentary, and no tools.\n\n")
	if len(recent) > 0 {
		b.WriteString("Recent branches of the repository:\n" + strings.Join(recent, "\n") + "\n\n")
	}
	b.WriteString("Task:\n" + strings.TrimSpace(task))
	return b.String()
}

// WriteBranch runs argv in dir with a branch's prompt (BranchPrompt) on
// its stdin, and returns the branch it named, with prefix (BranchName).
func WriteBranch(ctx context.Context, dir string, argv []string, prompt, prefix string) (string, error) {
	reply, err := write(ctx, dir, argv, prompt, func(s string) string { return BranchName(clean(s, branchOpen, branchClose), prefix) })
	if err != nil && strings.HasSuffix(err.Error(), "wrote no message") {
		err = errors.New(baseName(argv[0]) + " named no branch")
	}
	return reply, err
}

// BranchName is the branch a model's reply names, as a git branch's name
// may be: its first line, prefix before it, TypePlaceholder in prefix the
// kind of work the reply begins with ("feat" when it names none, and the
// reply's own without one), and a static prefix the reply has already
// once.
func BranchName(reply, prefix string) string {
	reply = strings.TrimSpace(reply)
	if i := strings.IndexByte(reply, '\n'); i >= 0 {
		reply = reply[:i]
	}
	reply = strings.Trim(strings.TrimSpace(reply), "`\"'*")
	prefix = strings.TrimSpace(prefix)
	if strings.Contains(prefix, TypePlaceholder) {
		kind := "feat"
		if t, rest, ok := strings.Cut(reply, "/"); ok && isWord(t) {
			kind, reply = strings.ToLower(t), rest
		}
		prefix = strings.ReplaceAll(prefix, TypePlaceholder, kind)
	} else if prefix != "" {
		reply = strings.TrimPrefix(reply, prefix)
	}
	name := RefName(reply)
	if name == "" {
		return ""
	}
	return RefName(prefix + name)
}

// BranchFallback is a branch named from the task's first words, as Kero
// names it when the model can't: prefix before them, TypePlaceholder in it
// "fix" for a task that says so, else "feat".
func BranchFallback(task, prefix string) string {
	var words []string
	kind := "feat"
	for _, w := range strings.FieldsFunc(strings.ToLower(task), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		switch w {
		case "fix", "bug", "broken", "crash", "error":
			kind = "fix"
		}
		if len(words) < 5 {
			words = append(words, w)
		}
	}
	slug := strings.Join(words, "-")
	if r := []rune(slug); len(r) > 40 {
		slug = strings.TrimRight(string(r[:40]), "-")
	}
	if slug == "" {
		slug = "task"
	}
	return RefName(strings.ReplaceAll(strings.TrimSpace(prefix), TypePlaceholder, kind) + slug)
}

// UniqueBranch is name, else name with the first number after it that
// taken doesn't take.
func UniqueBranch(name string, taken func(string) bool) string {
	if name == "" || !taken(name) {
		return name
	}
	for n := 2; ; n++ {
		if s := name + "-" + strconv.Itoa(n); !taken(s) {
			return s
		}
	}
}

// RefName is s as a git branch's name may be: spaces as hyphens, without
// what git refuses in one (git check-ref-format), as "..", "@{", a
// component's leading dot or a trailing ".lock", and at most 100
// characters.
func RefName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte('-')
		case unicode.IsControl(r) || strings.ContainsRune("~^:?*[\\", r):
		default:
			b.WriteRune(r)
		}
	}
	s = b.String()
	for _, bad := range []string{"..", "@{", "//", "--"} {
		for strings.Contains(s, bad) {
			s = strings.ReplaceAll(s, bad, bad[:1])
		}
	}
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
	}
	parts := strings.Split(s, "/")
	kept := parts[:0]
	for _, p := range parts {
		p = strings.TrimLeft(p, ".-")
		p = strings.TrimRight(p, ".-")
		for strings.HasSuffix(p, ".lock") {
			p = strings.TrimRight(strings.TrimSuffix(p, ".lock"), ".-")
		}
		if p != "" {
			kept = append(kept, p)
		}
	}
	s = strings.Join(kept, "/")
	if s == "@" {
		return ""
	}
	return s
}

// isWord tells s is letters alone, as a kind of work is.
func isWord(s string) bool {
	if s == "" || len(s) > 12 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
