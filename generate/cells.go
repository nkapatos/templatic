package generate

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

var cellCtors = map[string]int{"NewCell": 1, "DeclareSessionCell": 0}

type cellTopics struct {
	topics     map[string]string
	unresolved []string
}

func collectCellTopics(prog *program) *cellTopics {
	out := &cellTopics{topics: map[string]string{}}
	for _, c := range prog.calls {
		argIdx, ok := cellCtors[c.fn]
		if !ok || argIdx >= len(c.call.Args) {
			continue
		}
		if topic, ok := c.constString(argIdx); ok {
			if _, dup := out.topics[topic]; !dup {
				out.topics[topic] = c.where
			}
		} else {
			out.unresolved = append(out.unresolved, c.where)
		}
	}
	return out
}

func checkBinds(prog *program, islands []Island) error {
	if !slices.ContainsFunc(islands, func(isl Island) bool { return len(isl.Binds) > 0 }) {
		return nil
	}
	ct := collectCellTopics(prog)
	var errs []string
	for _, isl := range islands {
		for _, b := range isl.Binds {
			if _, ok := ct.topics[b.Topic]; ok {
				continue
			}
			msg := fmt.Sprintf("%s: island %s: bind=%s:%s names no declared cell (live.NewCell / live.DeclareSessionCell with a literal or const topic)",
				isl.File, isl.Tag, b.Field, b.Topic)
			if near := nearestTopic(b.Topic, ct.topics); near != "" {
				msg += fmt.Sprintf("; did you mean %q (%s)?", near, ct.topics[near])
			}
			if len(ct.unresolved) > 0 {
				msg += fmt.Sprintf("; computed topics the check cannot read: %s", strings.Join(ct.unresolved, ", "))
			}
			errs = append(errs, msg)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("generate: %s", strings.Join(errs, "\n"))
	}
	return nil
}

func nearestTopic(want string, topics map[string]string) string {
	best, bestD := "", len(want)/2+1
	for _, t := range slices.Sorted(maps.Keys(topics)) {
		if d := levenshtein(want, t); d < bestD {
			best, bestD = t, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
