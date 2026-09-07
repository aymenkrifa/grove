package discover

import (
	"fmt"
	"path"
	"strings"
)

// Select narrows repos by selector, using the four stages in the spec, §6.1.
// An empty selector returns everything. Ambiguity is an error, never a guess.
func Select(repos []Found, selector string) ([]Found, error) {
	if selector == "" {
		return repos, nil
	}
	sel := strings.TrimSuffix(strings.TrimPrefix(selector, "./"), "/")

	// 1. exact relative path
	for _, r := range repos {
		if r.RelPath == sel {
			return []Found{r}, nil
		}
	}
	// 2. exact group
	var group []Found
	for _, r := range repos {
		if r.Group == sel {
			group = append(group, r)
		}
	}
	if len(group) > 0 {
		return group, nil
	}
	// 3. exact basename
	var base []Found
	for _, r := range repos {
		if path.Base(r.RelPath) == sel {
			base = append(base, r)
		}
	}
	if len(base) == 1 {
		return base, nil
	}
	if len(base) > 1 {
		return nil, ambiguous(sel, base)
	}
	// 4. unique substring
	var sub []Found
	for _, r := range repos {
		if strings.Contains(r.RelPath, sel) {
			sub = append(sub, r)
		}
	}
	switch len(sub) {
	case 1:
		return sub, nil
	case 0:
		return nil, fmt.Errorf("no repository matches %q; `grove list` shows what is available", selector)
	default:
		return nil, ambiguous(sel, sub)
	}
}

func ambiguous(sel string, candidates []Found) error {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.RelPath
	}
	return fmt.Errorf("%q is ambiguous — it matches %s", sel, strings.Join(names, ", "))
}
