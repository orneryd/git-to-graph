package nornic

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
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

func (e *Exporter) Write(versions []ledger.FactVersion, events []ledger.MutationEvent) error {
	if e.BatchSize <= 0 {
		e.BatchSize = 500
	}
	if err := os.MkdirAll(e.OutDir, 0o755); err != nil {
		return err
	}
	if err := writeVersionBatches(filepath.Join(e.OutDir, "nornic_versions.cypher"), versions, e.BatchSize); err != nil {
		return err
	}
	if err := writeEventBatches(filepath.Join(e.OutDir, "nornic_events.cypher"), events, e.BatchSize); err != nil {
		return err
	}
	if err := writeBootstrapHint(filepath.Join(e.OutDir, "README.txt")); err != nil {
		return err
	}
	return nil
}

func writeVersionBatches(path string, versions []ledger.FactVersion, batch int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "// Canonical fact-version upserts for NornicDB")
	fmt.Fprintln(w, "// Run canonical-bootstrap.cypher first")
	_ = batch
	for _, v := range versions {
		validFromISO := v.ValidFrom.UTC().Format("2006-01-02T15:04:05Z")
		validToExpr := "null"
		if v.ValidTo != nil {
			validToExpr = fmt.Sprintf("datetime('%s')", v.ValidTo.UTC().Format("2006-01-02T15:04:05Z"))
		}
		versionID := factVersionID(v)
		subjectID, predicate := factKeyParts(v.FactKey)
		fmt.Fprintf(w, "MERGE (:FactKey {subject_entity_id: '%s', predicate: '%s'});\n", esc(subjectID), esc(predicate))
		// Keep MERGE patterns as plain literal key/value lookups for parser compatibility.
		fmt.Fprintf(w, "MERGE (:FactVersion {version_id: '%s'}) SET fact_key = '%s', tx_id = '%s', commit_hash = '%s', valid_from_iso = '%s', valid_from = datetime('%s'), value_json = '%s', valid_to = %s, asserted_at = datetime('%s'), asserted_by = '%s';\n",
			esc(versionID), esc(v.FactKey), esc(v.TxID), esc(v.CommitHash), validFromISO, validFromISO, esc(v.ValueJSON), validToExpr, v.AssertedAt.UTC().Format("2006-01-02T15:04:05Z"), esc(v.AssertedBy))
		fmt.Fprintf(w, "MATCH (fk:FactKey {subject_entity_id: '%s', predicate: '%s'}) MATCH (fv:FactVersion {version_id: '%s'}) MERGE (fk)-[:HAS_VERSION]->(fv);\n",
			esc(subjectID), esc(predicate), esc(versionID))
	}
	return nil
}

func writeEventBatches(path string, events []ledger.MutationEvent, batch int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "// Mutation events aligned to ledger versions")
	_ = batch
	for _, ev := range events {
		fmt.Fprintf(w, "MERGE (:MutationEvent {event_id: '%s'}) SET tx_id = '%s', actor = '%s', timestamp = datetime('%s'), op_type = '%s', commit_hash = '%s';\n",
			esc(ev.EventID), esc(ev.TxID), esc(ev.Actor), ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z"), esc(ev.OpType), esc(ev.CommitHash))
		fmt.Fprintf(w, "MATCH (me:MutationEvent {event_id: '%s'}) MATCH (fv:FactVersion {fact_key: '%s', tx_id: '%s'}) MERGE (me)-[:AFFECTS]->(fv);\n",
			esc(ev.EventID), esc(ev.AffectedFact), esc(ev.TxID))
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

func factVersionID(v ledger.FactVersion) string {
	validFromISO := v.ValidFrom.UTC().Format("2006-01-02T15:04:05Z")
	payload := strings.Join([]string{v.FactKey, v.TxID, v.CommitHash, validFromISO}, "|")
	sum := sha1.Sum([]byte(payload))
	return "fv-" + hex.EncodeToString(sum[:])
}
