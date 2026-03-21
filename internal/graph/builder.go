package graph

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/c815719/git-to-graph/internal/model"
)

func BuildFacts(repoID string, files map[string]model.FileGraph, callTargets map[string]string) map[string]string {
	facts := map[string]string{}
	localCallable := map[string]map[string]string{}
	for path, fg := range files {
		byName := map[string]string{}
		for _, sym := range fg.Symbols {
			if !isCallableKind(sym.Kind) {
				continue
			}
			if existing, ok := byName[sym.Name]; ok && existing != sym.ID {
				delete(byName, sym.Name)
				continue
			}
			byName[sym.Name] = sym.ID
		}
		localCallable[path] = byName
	}

	for path, fg := range files {
		facts[factKey("file", path)] = mustJSON(map[string]any{
			"repo": repoID,
			"path": path,
			"lang": fg.Lang,
		})

		for _, imp := range dedupeStrings(fg.Imports) {
			facts[factKey("import", path+"->"+imp)] = mustJSON(map[string]any{
				"repo":   repoID,
				"from":   path,
				"import": imp,
			})
		}

		for _, sym := range fg.Symbols {
			facts[factKey("symbol", sym.ID)] = mustJSON(map[string]any{
				"repo": repoID,
				"id":   sym.ID,
				"name": sym.Name,
				"kind": sym.Kind,
				"file": sym.FilePath,
				"lang": sym.Language,
			})
			facts[factKey("contains", path+"->"+sym.ID)] = mustJSON(map[string]any{
				"repo":   repoID,
				"source": path,
				"target": sym.ID,
			})
		}

		for _, call := range fg.Calls {
			calleeID, ok := callTargets[call.Callee]
			if !ok {
				if localID, localOK := localCallable[path][call.Callee]; localOK {
					calleeID = localID
				} else {
					continue
				}
			}
			facts[factKey("calls", call.Caller+"->"+calleeID)] = mustJSON(map[string]any{
				"repo":   repoID,
				"source": call.Caller,
				"target": calleeID,
			})
		}
	}

	return facts
}

func BuildNameIndex(files map[string]model.FileGraph) map[string]string {
	byName := map[string][]string{}
	for _, fg := range files {
		for _, sym := range fg.Symbols {
			if !isCallableKind(sym.Kind) {
				continue
			}
			byName[sym.Name] = append(byName[sym.Name], sym.ID)
		}
	}
	resolved := map[string]string{}
	for name, ids := range byName {
		sort.Strings(ids)
		if len(ids) == 1 {
			resolved[name] = ids[0]
		}
	}
	return resolved
}

func isCallableKind(kind string) bool {
	switch kind {
	case "function", "method", "class", "type":
		return true
	default:
		return false
	}
}

func factKey(kind, id string) string {
	return fmt.Sprintf("repo_fact|%s|%s", kind, id)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
