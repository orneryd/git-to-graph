package parser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/c815719/git-to-graph/internal/model"
	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
	tsjava "github.com/smacker/go-tree-sitter/java"
	tsjs "github.com/smacker/go-tree-sitter/javascript"
	tspy "github.com/smacker/go-tree-sitter/python"
	tstsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	tsts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

type Backend string

const (
	BackendAuto       Backend = "auto"
	BackendTreeSitter Backend = "tree-sitter"
	BackendSCIP       Backend = "scip"
	BackendRegex      Backend = "regex"
)

var forcedBackend Backend = BackendAuto

func SetBackendMode(mode string) {
	m := Backend(strings.ToLower(strings.TrimSpace(mode)))
	switch m {
	case BackendAuto, BackendTreeSitter, BackendSCIP, BackendRegex:
		forcedBackend = m
	default:
		forcedBackend = BackendAuto
	}

}

func identifierChildren(n *sitter.Node, src []byte) []string {
	count := int(n.ChildCount())
	out := make([]string, 0, count)
	seen := map[string]struct{}{}
	for i := 0; i < count; i++ {
		c := n.Child(i)
		t := c.Type()
		if t != "identifier" && t != "type_identifier" && t != "property_identifier" {
			continue
		}
		name := strings.TrimSpace(nodeText(c, src))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

var (
	importRe = regexp.MustCompile(`(?m)^\s*(?:import|from)\s+([^\s;]+)`)
	callRe   = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

var keywordCalls = map[string]struct{}{
	"if": {}, "for": {}, "while": {}, "switch": {}, "return": {}, "func": {}, "def": {}, "class": {}, "new": {}, "catch": {}, "with": {},
}

func Parse(path, content string) model.FileGraph {
	lang := detectLanguage(path)
	backend := selectBackend(path, lang)
	switch backend {
	case BackendTreeSitter, BackendSCIP:
		if fg, ok := parseTreeSitter(path, content, lang); ok {
			return fg
		}
	}
	return parseRegex(path, content, lang)
}

func selectBackend(path, lang string) Backend {
	mode := string(forcedBackend)
	if mode == "" || mode == string(BackendAuto) {
		mode = strings.ToLower(strings.TrimSpace(getEnv("GIT_TO_GRAPH_PARSER_BACKEND", "auto")))
	}
	_ = path
	if mode == string(BackendSCIP) {
		if scipSupportedAndInstalled(lang) {
			return BackendSCIP
		}
		return BackendTreeSitter
	}
	if mode == string(BackendTreeSitter) {
		return BackendTreeSitter
	}
	if mode == string(BackendRegex) {
		return BackendRegex
	}
	if scipSupportedAndInstalled(lang) {
		return BackendSCIP
	}
	return BackendTreeSitter
}

func scipSupportedAndInstalled(lang string) bool {
	binaryByLang := map[string]string{
		"python":     "scip-python",
		"typescript": "scip-typescript",
		"javascript": "scip-typescript",
		"go":         "scip-go",
		"rust":       "scip-rust",
		"java":       "scip-java",
	}
	b, ok := binaryByLang[lang]
	if !ok {
		return false
	}
	_, err := exec.LookPath(b)
	return err == nil
}

func parseTreeSitter(path, content, lang string) (fg model.FileGraph, ok bool) {
	fg = model.FileGraph{Path: path, Lang: lang}
	defer func() {
		if recover() != nil {
			fg = model.FileGraph{}
			ok = false
		}
	}()

	langSpec := languageFor(lang)
	if langSpec == nil {
		return model.FileGraph{}, false
	}

	p := sitter.NewParser()
	if p == nil {
		return model.FileGraph{}, false
	}
	p.SetLanguage(langSpec)
	ctx, cancel := context.WithTimeout(context.TODO(), parseTimeout())
	defer cancel()
	tree, err := p.ParseCtx(ctx, nil, []byte(content))
	if err != nil || tree == nil {
		return model.FileGraph{}, false
	}
	defer tree.Close()
	root := tree.RootNode()
	walkTree(root, []byte(content), func(n *sitter.Node) {
		t := n.Type()
		switch t {
		case "function_declaration", "function_definition", "method_definition", "method_declaration":
			name := nodeFieldText(n, "name", []byte(content))
			if name == "" {
				name = firstIdentifierChild(n, []byte(content))
			}
			if name != "" {
				kind := "function"
				if strings.Contains(t, "method") {
					kind = "method"
				}
				fg.Symbols = append(fg.Symbols, model.Symbol{ID: symbolID(path, name, kind), Name: name, Kind: kind, FilePath: path, Language: lang})
			}
		case "class_declaration", "class_definition":
			name := nodeFieldText(n, "name", []byte(content))
			if name == "" {
				name = firstIdentifierChild(n, []byte(content))
			}
			if name != "" {
				fg.Symbols = append(fg.Symbols, model.Symbol{ID: symbolID(path, name, "class"), Name: name, Kind: "class", FilePath: path, Language: lang})
			}
		case "type_declaration", "type_spec":
			name := firstIdentifierChild(n, []byte(content))
			if name != "" {
				fg.Symbols = append(fg.Symbols, model.Symbol{ID: symbolID(path, name, "type"), Name: name, Kind: "type", FilePath: path, Language: lang})
			}
		case "const_declaration", "const_spec", "constant_declaration":
			for _, name := range identifierChildren(n, []byte(content)) {
				fg.Symbols = append(fg.Symbols, model.Symbol{ID: symbolID(path, name, "constant"), Name: name, Kind: "constant", FilePath: path, Language: lang})
			}
		case "var_declaration", "var_spec", "variable_declaration", "lexical_declaration":
			for _, name := range identifierChildren(n, []byte(content)) {
				fg.Symbols = append(fg.Symbols, model.Symbol{ID: symbolID(path, name, "variable"), Name: name, Kind: "variable", FilePath: path, Language: lang})
			}
		case "import_declaration", "import_statement":
			v := strings.TrimSpace(nodeText(n, []byte(content)))
			if v != "" {
				fg.Imports = append(fg.Imports, v)
			}
		case "call_expression", "invocation_expression":
			callee := nodeFieldText(n, "function", []byte(content))
			if callee == "" {
				callee = firstIdentifierChild(n, []byte(content))
			}
			callee = normalizeCall(callee)
			if callee != "" {
				if _, skip := keywordCalls[callee]; !skip {
					fg.Calls = append(fg.Calls, model.CallEdge{Caller: "file::" + path, Callee: callee})
				}
			}
		}
	})

	if len(fg.Imports) == 0 {
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				fg.Imports = append(fg.Imports, strings.TrimSpace(m[1]))
			}
		}
	}
	return fg, true
}

func parseRegex(path, content, lang string) model.FileGraph {
	fg := model.FileGraph{Path: path, Lang: lang}
	for _, m := range importRe.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			fg.Imports = append(fg.Imports, strings.TrimSpace(m[1]))
		}
	}
	for _, m := range callRe.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		name := m[1]
		if _, skip := keywordCalls[name]; skip {
			continue
		}
		fg.Calls = append(fg.Calls, model.CallEdge{Caller: "file::" + path, Callee: name})
	}
	return fg
}

func languageFor(lang string) *sitter.Language {
	switch lang {
	case "go":
		return tsgo.GetLanguage()
	case "python":
		return tspy.GetLanguage()
	case "javascript":
		return tsjs.GetLanguage()
	case "typescript":
		return tsts.GetLanguage()
	case "tsx":
		return tstsx.GetLanguage()
	case "java":
		return tsjava.GetLanguage()
	default:
		return nil
	}
}

func walkTree(n *sitter.Node, src []byte, visit func(*sitter.Node)) {
	if n == nil {
		return
	}
	visit(n)
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		walkTree(n.Child(i), src, visit)
	}
}

func nodeFieldText(n *sitter.Node, field string, src []byte) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return nodeText(c, src)
}

func nodeText(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(string(src[n.StartByte():n.EndByte()]))
}

func firstIdentifierChild(n *sitter.Node, src []byte) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		c := n.Child(i)
		t := c.Type()
		if t == "identifier" || t == "type_identifier" || t == "property_identifier" {
			return nodeText(c, src)
		}
	}
	return ""
}

func normalizeCall(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.Contains(v, ".") {
		parts := strings.Split(v, ".")
		v = parts[len(parts)-1]
	}
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, "(")
	return v
}

func detectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".java":
		return "java"
	default:
		return "unknown"
	}
}

func symbolID(path, name, kind string) string {
	return "symbol::" + path + "::" + kind + "::" + name
}

func getEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func parseTimeout() time.Duration {
	v := strings.TrimSpace(os.Getenv("G2G_PARSE_TIMEOUT_MS"))
	if v == "" {
		return 1500 * time.Millisecond
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		_ = fmt.Sprintf("invalid G2G_PARSE_TIMEOUT_MS=%q", v)
		return 1500 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
