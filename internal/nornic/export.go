package nornic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c815719/git-to-graph/internal/ledger"
)

type Exporter struct {
	OutDir    string
	BatchSize int
}

func (e *Exporter) Write(states []ledger.CodeState, changes []ledger.CodeChange) error {
	if e.BatchSize <= 0 {
		e.BatchSize = 500
	}
	if err := os.MkdirAll(e.OutDir, 0o755); err != nil {
		return err
	}
	if err := writeVersionBatches(filepath.Join(e.OutDir, "nornic_versions.cypher"), states, e.BatchSize); err != nil {
		return err
	}
	if err := writeEventBatches(filepath.Join(e.OutDir, "nornic_events.cypher"), changes, e.BatchSize); err != nil {
		return err
	}
	if err := writeBootstrapHint(filepath.Join(e.OutDir, "README.txt")); err != nil {
		return err
	}
	return nil
}

func writeVersionBatches(path string, versions []ledger.CodeState, batch int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "// Canonical code-state upserts for NornicDB")
	fmt.Fprintln(w, "// Run canonical-bootstrap.cypher first")
	_ = batch
	lastStateByCodeKey := map[string]string{}
	for _, v := range versions {
		validFromISO := v.ValidFrom.UTC().Format("2006-01-02T15:04:05Z")
		validToExpr := "null"
		if v.ValidTo != nil {
			validToExpr = fmt.Sprintf("datetime('%s')", v.ValidTo.UTC().Format("2006-01-02T15:04:05Z"))
		}
		stateID := v.StateID()
		subjectID, predicate := factKeyParts(v.CodeKey)
		keyLabel, versionLabel := semanticLabels(predicate, v.ValueJSON)
		fmt.Fprintf(w, "MERGE (:CodeKey:%s {entity_id: '%s', relation_type: '%s'});\n", keyLabel, esc(subjectID), esc(predicate))
		// Keep MERGE patterns as plain literal key/value lookups for parser compatibility.
		fmt.Fprintf(w, "MERGE (:CodeState:%s {state_id: '%s'}) SET code_key = '%s', tx_id = '%s', commit_hash = '%s', valid_from_iso = '%s', valid_from = datetime('%s'), value_json = '%s', valid_to = %s, asserted_at = datetime('%s'), asserted_by = '%s', semantic_type = '%s';\n",
			esc(versionLabel),
			esc(stateID), esc(v.CodeKey), esc(v.TxID), esc(v.CommitHash), validFromISO, validFromISO, esc(v.ValueJSON), validToExpr, v.AssertedAt.UTC().Format("2006-01-02T15:04:05Z"), esc(v.AssertedBy), esc(predicate))
		fmt.Fprintf(w, "MATCH (ck:CodeKey {entity_id: '%s', relation_type: '%s'}) MATCH (cs:CodeState {state_id: '%s'}) MERGE (ck)-[:HAS_STATE]->(cs);\n",
			esc(subjectID), esc(predicate), esc(stateID))
		fmt.Fprintf(w, "MERGE (:Commit {hash: '%s'}) SET timestamp = datetime('%s'), tx_id = '%s', actor = '%s';\n",
			esc(v.CommitHash), v.AssertedAt.UTC().Format("2006-01-02T15:04:05Z"), esc(v.TxID), esc(v.AssertedBy))
		fmt.Fprintf(w, "MATCH (c:Commit {hash: '%s'}) MATCH (cs:CodeState {state_id: '%s'}) MERGE (c)-[:CHANGED]->(cs);\n",
			esc(v.CommitHash), esc(stateID))
		fmt.Fprintf(w, "MATCH (c:Commit {hash: '%s'}) MATCH (ck:CodeKey {entity_id: '%s', relation_type: '%s'}) MERGE (c)-[:TOUCHED]->(ck);\n",
			esc(v.CommitHash), esc(subjectID), esc(predicate))
		if prevStateID, ok := lastStateByCodeKey[v.CodeKey]; ok {
			fmt.Fprintf(w, "MATCH (prev:CodeState {state_id: '%s'}) MATCH (curr:CodeState {state_id: '%s'}) MERGE (prev)-[:SUPERSEDED_BY]->(curr);\n",
				esc(prevStateID), esc(stateID))
		}
		lastStateByCodeKey[v.CodeKey] = stateID
	}
	return nil
}

func writeEventBatches(path string, events []ledger.CodeChange, batch int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "// Code change events aligned to code states")
	_ = batch
	for _, ev := range events {
		affectedStateID := ev.AffectedStateID
		if strings.TrimSpace(affectedStateID) == "" {
			affectedStateID = "missing"
		}
		fmt.Fprintf(w, "MERGE (:CodeChange {change_id: '%s'}) SET tx_id = '%s', actor = '%s', timestamp = datetime('%s'), op_type = '%s', commit_hash = '%s';\n",
			esc(ev.ChangeID), esc(ev.TxID), esc(ev.Actor), ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z"), esc(ev.OpType), esc(ev.CommitHash))
		fmt.Fprintf(w, "MERGE (:Commit {hash: '%s'}) SET timestamp = datetime('%s'), tx_id = '%s', actor = '%s';\n",
			esc(ev.CommitHash), ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z"), esc(ev.TxID), esc(ev.Actor))
		fmt.Fprintf(w, "MATCH (c:Commit {hash: '%s'}) MATCH (cc:CodeChange {change_id: '%s'}) MERGE (c)-[:EMITTED]->(cc);\n",
			esc(ev.CommitHash), esc(ev.ChangeID))
		fmt.Fprintf(w, "MATCH (cc:CodeChange {change_id: '%s'}) MATCH (cs:CodeState {state_id: '%s'}) MERGE (cc)-[:IMPACTS]->(cs);\n",
			esc(ev.ChangeID), esc(affectedStateID))
	}
	return nil
}

func writeBootstrapHint(path string) error {
	content := strings.Join([]string{
		"Generated NornicDB artifacts:",
		"1) Run your canonical bootstrap schema first.",
		"2) Execute nornic_versions.cypher, then nornic_events.cypher.",
		"3) Query temporal state with db.temporal.asOf(...).",
		"4) Link tx_id to db.txlog.byTxId(tx_id, ... ) for receipts/audit.",
	}, "\n")
	return os.WriteFile(path, []byte(content+"\n"), 0o644)
}

func esc(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "'", "\\'")
	v = strings.ReplaceAll(v, "\n", "\\n")
	v = strings.ReplaceAll(v, "\r", "")
	return v
}

func factKeyParts(factKey string) (subjectID, predicate string) {
	parts := strings.Split(factKey, "|")
	if len(parts) >= 3 {
		return parts[2], parts[1]
	}
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return factKey, "unknown"
}

func semanticLabels(predicate, valueJSON string) (keyLabel, versionLabel string) {
	switch strings.ToLower(strings.TrimSpace(predicate)) {
	case "repository":
		return "RepositoryKey", "RepositoryState"
	case "directory":
		return "DirectoryKey", "DirectoryState"
	case "module":
		return "ModuleKey", "ModuleState"
	case "file":
		return "CodeFileKey", "CodeFileState"
	case "function":
		return "CodeFunctionKey", "FunctionSymbolState"
	case "method":
		return "CodeMethodKey", "MethodSymbolState"
	case "class":
		return "CodeClassKey", "ClassSymbolState"
	case "interface":
		return "CodeInterfaceKey", "InterfaceSymbolState"
	case "trait":
		return "CodeTraitKey", "TraitSymbolState"
	case "macro":
		return "CodeMacroKey", "MacroSymbolState"
	case "struct":
		return "CodeStructKey", "StructSymbolState"
	case "enum":
		return "CodeEnumKey", "EnumSymbolState"
	case "union":
		return "CodeUnionKey", "UnionSymbolState"
	case "record":
		return "CodeRecordKey", "RecordSymbolState"
	case "property":
		return "CodePropertyKey", "PropertySymbolState"
	case "annotation":
		return "CodeAnnotationKey", "AnnotationSymbolState"
	case "variable":
		return "CodeVariableKey", "VariableSymbolState"
	case "constant":
		return "CodeConstantKey", "ConstantSymbolState"
	case "type":
		return "CodeTypeKey", "TypeSymbolState"
	case "symbol":
		switch strings.ToLower(strings.TrimSpace(symbolKindFromValue(valueJSON))) {
		case "function":
			return "CodeSymbolKey", "FunctionSymbolState"
		case "method":
			return "CodeSymbolKey", "MethodSymbolState"
		case "class":
			return "CodeSymbolKey", "ClassSymbolState"
		case "type":
			return "CodeSymbolKey", "TypeSymbolState"
		case "constant":
			return "CodeSymbolKey", "ConstantSymbolState"
		case "variable":
			return "CodeSymbolKey", "VariableSymbolState"
		default:
			return "CodeSymbolKey", "CodeSymbolState"
		}
	case "calls":
		return "CallEdgeKey", "CallEdgeState"
	case "contains":
		return "ContainsEdgeKey", "ContainsEdgeState"
	case "imports", "import":
		return "ImportEdgeKey", "ImportEdgeState"
	case "inherits":
		return "InheritsEdgeKey", "InheritsEdgeState"
	default:
		return "CodeEntityKey", "CodeEntityState"
	}
}

func symbolKindFromValue(valueJSON string) string {
	if strings.TrimSpace(valueJSON) == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(valueJSON), &payload); err != nil {
		return ""
	}
	v, _ := payload["kind"].(string)
	return v
}
