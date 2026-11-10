package settings

import (
	"fmt"
	"strings"
)

// A setting's value as text, for kero config get and set to give an agent
// a prompt as it reads, not as the file escapes it; one repository's line
// of a key that holds a line a repository, leaving the others; and Kero's
// own prompt where the setting is empty, which is not in Settings but in
// the package that uses it.

// builtins are Kero's own texts of the text keys whose default, "", stands
// for one, by key: the packages that have them register them
// (SetBuiltin), as settings imports none of them.
var builtins = map[string]func() string{}

// SetBuiltin registers Kero's own text of a text key, which "" stands for.
func SetBuiltin(name string, text func() string) {
	builtins[name] = text
}

// Value is a setting's value: a text key's as its lines, not as the file
// escapes them, else as kero config get prints it, a list's a line each.
// repo, of a key that holds a line a repository (review-prompt), is that
// repository's value alone. builtin gives Kero's own value in place of the
// user's: of a text key whose default is "", Kero's own text.
func Value(name, repo string, builtin bool) (string, error) {
	k, repo, err := valueKey(name, repo)
	if err != nil {
		return "", err
	}
	s := Load()
	if builtin {
		if repo != "" {
			return "", fmt.Errorf("%s: a repository's value has no default of its own, it falls back on the setting for every repository", name)
		}
		s = Default()
	}
	vals := k.get(&s)
	if repo != "" {
		for _, line := range vals {
			if r, v, _ := strings.Cut(line, "="); r == repo {
				return unescapeText(v), nil
			}
		}
		return "", nil
	}
	if k.text && !k.list {
		v := unescapeText(strings.Join(vals, ""))
		if f := builtins[name]; builtin && v == "" && f != nil {
			v = f()
		}
		return v, nil
	}
	return strings.Join(vals, "\n"), nil
}

// SetValue sets a setting to value, as Value gives it, and saves the file:
// a text key's lines as they are; a list's a line each; "" for its
// default. repo sets that repository's value alone, "" to remove it, and
// leaves the other repositories' as they are.
func SetValue(name, repo, value string) error {
	k, repo, err := valueKey(name, repo)
	if err != nil {
		return err
	}
	switch {
	case repo != "":
		s := Load()
		var lines []string
		for _, line := range k.get(&s) {
			if r, _, _ := strings.Cut(line, "="); r != repo {
				lines = append(lines, line)
			}
		}
		if strings.TrimSpace(value) != "" {
			lines = append(lines, repo+"="+escapeText(value))
		}
		return Set(name, lines)
	case k.text && !k.list:
		return Set(name, []string{escapeText(value)})
	case k.list:
		var lines []string
		for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		return Set(name, lines)
	case strings.TrimSpace(value) == "":
		return Set(name, nil)
	}
	return Set(name, []string{strings.TrimSpace(value)})
}

// valueKey is the key name, and repo as a repository's line names it.
func valueKey(name, repo string) (*key, string, error) {
	k := findKey(name)
	if k == nil {
		return nil, "", fmt.Errorf("no setting %q (kero config show lists them)", name)
	}
	repo = strings.ToLower(strings.TrimSpace(repo))
	if repo != "" && !k.repo {
		return nil, "", fmt.Errorf("%s is no repository's setting: it takes no repository", name)
	}
	if repo != "" && !strings.Contains(repo, "/") {
		return nil, "", fmt.Errorf("%q is not <owner>/<name>", repo)
	}
	return k, repo, nil
}
