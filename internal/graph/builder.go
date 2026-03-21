package graph

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/c815719/git-to-graph/internal/model"
)

func BuildFacts(repoID string, files map[string]model.FileGraph, callTargets map[string]string) map[string]string {
	facts := map[string]string{}
	facts[factKey("repository", repoID)] = mustJSON(map[string]any{
		"name": repoID,
	})

	directorySet := map[string]struct{}{}
	for path := range files {
		dir := filepath.Clean(filepath.Dir(path))
		for dir != "." && dir != string(filepath.Separator) {
			directorySet[dir] = struct{}{}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	for dir := range directorySet {
		facts[factKey("directory", dir)] = mustJSON(map[string]any{
			"repo": repoID,
			"path": dir,
			"name": filepath.Base(dir),
		})
		parent := filepath.Dir(dir)
		if parent == "." || parent == string(filepath.Separator) || parent == dir {
			facts[factKey("contains", repoID+"->"+dir)] = mustJSON(map[string]any{
				"repo":        repoID,
				"source":      repoID,
				"source_type": "repository",
				"target":      dir,
				"target_type": "directory",
			})
		} else {
			facts[factKey("contains", parent+"->"+dir)] = mustJSON(map[string]any{
				"repo":        repoID,
				"source":      parent,
				"source_type": "directory",
				"target":      dir,
				"target_type": "directory",
			})
		}
	}
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
			"name": filepath.Base(path),
		})
		dir := filepath.Clean(filepath.Dir(path))
		if dir != "." && dir != string(filepath.Separator) {
			facts[factKey("contains", dir+"->"+path)] = mustJSON(map[string]any{
				"repo":        repoID,
				"source":      dir,
				"source_type": "directory",
				"target":      path,
				"target_type": "file",
			})
		} else {
			facts[factKey("contains", repoID+"->"+path)] = mustJSON(map[string]any{
				"repo":        repoID,
				"source":      repoID,
				"source_type": "repository",
				"target":      path,
				"target_type": "file",
			})
		}

		for _, imp := range dedupeStrings(fg.Imports) {
			facts[factKey("module", imp)] = mustJSON(map[string]any{
				"repo": repoID,
				"name": imp,
			})
			facts[factKey("imports", path+"->"+imp)] = mustJSON(map[string]any{
				"repo":          repoID,
				"source":        path,
				"source_type":   "file",
				"target":        imp,
				"target_type":   "module",
				"imported_name": imp,
			})
		}

		for _, sym := range fg.Symbols {
			symbolPredicate := symbolPredicate(sym.Kind)
			facts[factKey(symbolPredicate, sym.ID)] = mustJSON(map[string]any{
				"repo":        repoID,
				"id":          sym.ID,
				"name":        sym.Name,
				"kind":        sym.Kind,
				"file":        sym.FilePath,
				"lang":        sym.Language,
				"line_number": sym.Line,
			})
			facts[factKey("contains", path+"->"+sym.ID)] = mustJSON(map[string]any{
				"repo":        repoID,
				"source":      path,
				"source_type": "file",
				"target":      sym.ID,
				"target_type": "symbol",
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
				"repo":           repoID,
				"source":         call.Caller,
				"target":         calleeID,
				"line_number":    call.Line,
				"args":           call.Args,
				"full_call_name": strings.TrimSpace(call.FullName),
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

func symbolPredicate(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "function", "method", "class", "interface", "trait", "macro", "struct", "enum", "union", "record", "property", "annotation", "variable", "constant", "type":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return "symbol"
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
