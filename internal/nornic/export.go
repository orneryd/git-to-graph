package nornic

import (
	"bufio"
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

	fmt.Fprintln(w, "// Batched canonical fact-version upserts for NornicDB")
	fmt.Fprintln(w, "// Run canonical-bootstrap.cypher first")
	for i := 0; i < len(versions); i += batch {
		end := i + batch
		if end > len(versions) {
			end = len(versions)
		}
		chunk := versions[i:end]
		fmt.Fprintln(w, "UNWIND [")
		for idx, v := range chunk {
			comma := ","
			if idx == len(chunk)-1 {
				comma = ""
			}
			validTo := "null"
			if v.ValidTo != nil {
				validTo = fmt.Sprintf("datetime('%s')", v.ValidTo.UTC().Format("2006-01-02T15:04:05Z"))
			}
			fmt.Fprintf(w, "  {fact_key: '%s', value_json: '%s', valid_from: datetime('%s'), valid_to: %s, asserted_at: datetime('%s'), asserted_by: '%s', tx_id: '%s', commit_hash: '%s'}%s\n",
				esc(v.FactKey), esc(v.ValueJSON), v.ValidFrom.UTC().Format("2006-01-02T15:04:05Z"), validTo, v.AssertedAt.UTC().Format("2006-01-02T15:04:05Z"), esc(v.AssertedBy), esc(v.TxID), esc(v.CommitHash), comma)
		}
		fmt.Fprintln(w, "] AS row")
		fmt.Fprintln(w, "MERGE (fk:FactKey {subject_entity_id: split(row.fact_key,'|')[2], predicate: split(row.fact_key,'|')[1]})")
		fmt.Fprintln(w, "CREATE (fv:FactVersion {fact_key: row.fact_key, value_json: row.value_json, valid_from: row.valid_from, valid_to: row.valid_to, asserted_at: row.asserted_at, asserted_by: row.asserted_by, tx_id: row.tx_id, commit_hash: row.commit_hash})")
		fmt.Fprintln(w, "MERGE (fk)-[:HAS_VERSION]->(fv);")
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
	for i := 0; i < len(events); i += batch {
		end := i + batch
		if end > len(events) {
			end = len(events)
		}
		chunk := events[i:end]
		fmt.Fprintln(w, "UNWIND [")
		for idx, ev := range chunk {
			comma := ","
			if idx == len(chunk)-1 {
				comma = ""
			}
			fmt.Fprintf(w, "  {event_id: '%s', tx_id: '%s', actor: '%s', ts: datetime('%s'), op_type: '%s', commit_hash: '%s', affected_fact: '%s'}%s\n",
				esc(ev.EventID), esc(ev.TxID), esc(ev.Actor), ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z"), esc(ev.OpType), esc(ev.CommitHash), esc(ev.AffectedFact), comma)
		}
		fmt.Fprintln(w, "] AS row")
		fmt.Fprintln(w, "CREATE (me:MutationEvent {event_id: row.event_id, tx_id: row.tx_id, actor: row.actor, timestamp: row.ts, op_type: row.op_type, commit_hash: row.commit_hash})")
		fmt.Fprintln(w, "WITH me, row")
		fmt.Fprintln(w, "MATCH (fv:FactVersion {fact_key: row.affected_fact, tx_id: row.tx_id})")
		fmt.Fprintln(w, "MERGE (me)-[:AFFECTS]->(fv);")
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
