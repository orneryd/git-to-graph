package nornic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/c815719/git-to-graph/internal/ledger"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type BoltConfig struct {
	URI             string
	User            string
	Password        string
	Token           string
	Database        string
	ContinueOnError bool
}

func ApplyLedgerBolt(ctx context.Context, cfg BoltConfig, versions []ledger.CodeState, events []ledger.CodeChange, progress ApplyProgressFunc) (int, error) {
	if cfg.URI == "" {
		cfg.URI = "bolt://localhost:7687"
	}

	auth := neo4j.NoAuth()
	if strings.TrimSpace(cfg.Token) != "" {
		auth = neo4j.BearerAuth(cfg.Token)
	} else if strings.TrimSpace(cfg.User) != "" {
		auth = neo4j.BasicAuth(cfg.User, cfg.Password, "")
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return 0, err
	}
	defer driver.Close(ctx)

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return 0, err
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: cfg.Database})
	defer session.Close(ctx)

	batch := dbBatchSize()
	timeout := statementTimeout()
	total := len(versions) + len(events)
	done := 0

	const qVersions = `UNWIND $rows AS row
MERGE (ck:CodeKey {entity_id: row.entity_id, relation_type: row.relation_type})
MERGE (cs:CodeState {state_id: row.state_id})
SET cs.code_key = row.code_key,
    cs.tx_id = row.tx_id,
    cs.commit_hash = row.commit_hash,
    cs.valid_from_iso = row.valid_from_iso,
    cs.valid_from = datetime(row.valid_from_iso),
    cs.value_json = row.value_json,
    cs.valid_to = CASE WHEN row.valid_to_iso IS NULL THEN null ELSE datetime(row.valid_to_iso) END,
    cs.asserted_at = datetime(row.asserted_at_iso),
    cs.asserted_by = row.asserted_by,
    cs.semantic_type = row.semantic_type
MERGE (ck)-[:HAS_STATE]->(cs)
MERGE (c:Commit {hash: row.commit_hash})
ON CREATE SET c.timestamp = datetime(row.asserted_at_iso), c.tx_id = row.tx_id, c.actor = row.asserted_by
MERGE (c)-[:CHANGED]->(cs)
MERGE (c)-[:TOUCHED]->(ck)`
	const qVersionRow = `MERGE (ck:CodeKey {entity_id: $entity_id, relation_type: $relation_type})
MERGE (cs:CodeState {state_id: $state_id})
SET cs.code_key = $code_key,
    cs.tx_id = $tx_id,
    cs.commit_hash = $commit_hash,
    cs.valid_from_iso = $valid_from_iso,
    cs.valid_from = datetime($valid_from_iso),
    cs.value_json = $value_json,
    cs.valid_to = CASE WHEN $valid_to_iso IS NULL THEN null ELSE datetime($valid_to_iso) END,
    cs.asserted_at = datetime($asserted_at_iso),
    cs.asserted_by = $asserted_by,
    cs.semantic_type = $semantic_type
MERGE (ck)-[:HAS_STATE]->(cs)
MERGE (c:Commit {hash: $commit_hash})
ON CREATE SET c.timestamp = datetime($asserted_at_iso), c.tx_id = $tx_id, c.actor = $asserted_by
MERGE (c)-[:CHANGED]->(cs)
MERGE (c)-[:TOUCHED]->(ck)`

	for i := 0; i < len(versions); i += batch {
		end := i + batch
		if end > len(versions) {
			end = len(versions)
		}
		rows := make([]map[string]any, 0, end-i)
		for _, v := range versions[i:end] {
			subjectID, predicate := factKeyParts(v.CodeKey)
			row := map[string]any{
				"entity_id":       subjectID,
				"relation_type":   predicate,
				"state_id":        v.StateID(),
				"code_key":        v.CodeKey,
				"tx_id":           v.TxID,
				"commit_hash":     v.CommitHash,
				"valid_from_iso":  v.ValidFrom.UTC().Format("2006-01-02T15:04:05Z"),
				"asserted_at_iso": v.AssertedAt.UTC().Format("2006-01-02T15:04:05Z"),
				"asserted_by":     v.AssertedBy,
				"value_json":      v.ValueJSON,
				"semantic_type":   predicate,
				"valid_to_iso":    nil,
			}
			if v.ValidTo != nil {
				row["valid_to_iso"] = v.ValidTo.UTC().Format("2006-01-02T15:04:05Z")
			}
			rows = append(rows, row)
		}
		stmtCtx, cancel := context.WithTimeout(ctx, timeout)
		_, err := session.ExecuteWrite(stmtCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(stmtCtx, qVersions, map[string]any{"rows": rows})
			if err != nil {
				return nil, err
			}
			_, err = res.Consume(stmtCtx)
			return nil, err
		})
		cancel()
		if err != nil {
			// Fallback: some Nornic builds reject this UNWIND mutation shape.
			for j, row := range rows {
				rowCtx, rowCancel := context.WithTimeout(ctx, timeout)
				_, rowErr := session.ExecuteWrite(rowCtx, func(tx neo4j.ManagedTransaction) (any, error) {
					res, runErr := tx.Run(rowCtx, qVersionRow, row)
					if runErr != nil {
						return nil, runErr
					}
					_, consumeErr := res.Consume(rowCtx)
					return nil, consumeErr
				})
				rowCancel()
				if rowErr != nil {
					return done, fmt.Errorf("apply version batch [%d:%d] (row fallback failed at %d): %w", i, end, i+j, rowErr)
				}
				done++
				if progress != nil {
					progress(done, total, "versions")
				}
			}
			continue
		}
		done += len(rows)
		if progress != nil {
			progress(done, total, "versions")
		}
	}

	const qEvents = `UNWIND $rows AS row
MERGE (cc:CodeChange {change_id: row.change_id})
SET cc.tx_id = row.tx_id,
    cc.actor = row.actor,
    cc.timestamp = datetime(row.timestamp_iso),
    cc.op_type = row.op_type,
    cc.commit_hash = row.commit_hash
MERGE (c:Commit {hash: row.commit_hash})
ON CREATE SET c.timestamp = datetime(row.timestamp_iso), c.tx_id = row.tx_id, c.actor = row.actor
MERGE (c)-[:EMITTED]->(cc)
WITH cc, row
MATCH (cs:CodeState {state_id: row.affected_state_id})
MERGE (cc)-[:IMPACTS]->(cs)`
	const qEventRow = `MERGE (cc:CodeChange {change_id: $change_id})
SET cc.tx_id = $tx_id,
    cc.actor = $actor,
    cc.timestamp = datetime($timestamp_iso),
    cc.op_type = $op_type,
    cc.commit_hash = $commit_hash
MERGE (c:Commit {hash: $commit_hash})
ON CREATE SET c.timestamp = datetime($timestamp_iso), c.tx_id = $tx_id, c.actor = $actor
MERGE (c)-[:EMITTED]->(cc)
MATCH (cs:CodeState {state_id: $affected_state_id})
MERGE (cc)-[:IMPACTS]->(cs)`

	for i := 0; i < len(events); i += batch {
		end := i + batch
		if end > len(events) {
			end = len(events)
		}
		rows := make([]map[string]any, 0, end-i)
		for _, ev := range events[i:end] {
			affectedStateID := ev.AffectedStateID
			if strings.TrimSpace(affectedStateID) == "" {
				affectedStateID = "missing"
			}
			rows = append(rows, map[string]any{
				"change_id":         ev.ChangeID,
				"tx_id":             ev.TxID,
				"actor":             ev.Actor,
				"timestamp_iso":     ev.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
				"op_type":           ev.OpType,
				"commit_hash":       ev.CommitHash,
				"affected_state_id": affectedStateID,
			})
		}
		stmtCtx, cancel := context.WithTimeout(ctx, timeout)
		_, err := session.ExecuteWrite(stmtCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(stmtCtx, qEvents, map[string]any{"rows": rows})
			if err != nil {
				return nil, err
			}
			_, err = res.Consume(stmtCtx)
			return nil, err
		})
		cancel()
		if err != nil {
			// Fallback: execute event upserts row-by-row when UNWIND mutation is rejected.
			for j, row := range rows {
				rowCtx, rowCancel := context.WithTimeout(ctx, timeout)
				_, rowErr := session.ExecuteWrite(rowCtx, func(tx neo4j.ManagedTransaction) (any, error) {
					res, runErr := tx.Run(rowCtx, qEventRow, row)
					if runErr != nil {
						return nil, runErr
					}
					_, consumeErr := res.Consume(rowCtx)
					return nil, consumeErr
				})
				rowCancel()
				if rowErr != nil {
					return done, fmt.Errorf("apply event batch [%d:%d] (row fallback failed at %d): %w", i, end, i+j, rowErr)
				}
				done++
				if progress != nil {
					progress(done, total, "events")
				}
			}
			continue
		}
		done += len(rows)
		if progress != nil {
			progress(done, total, "events")
		}
	}

	return done, nil
}

func ApplyCypherFilesBolt(ctx context.Context, cfg BoltConfig, paths []string, progress ApplyProgressFunc) (int, error) {
	if cfg.URI == "" {
		cfg.URI = "bolt://localhost:7687"
	}

	auth := neo4j.NoAuth()
	if strings.TrimSpace(cfg.Token) != "" {
		auth = neo4j.BearerAuth(cfg.Token)
	} else if strings.TrimSpace(cfg.User) != "" {
		auth = neo4j.BasicAuth(cfg.User, cfg.Password, "")
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return 0, err
	}
	defer driver.Close(ctx)

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return 0, err
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: cfg.Database})
	defer session.Close(ctx)

	tasks, err := buildStatementTasks(paths)
	if err != nil {
		return 0, err
	}
	total := len(tasks)
	done := 0
	timeout := statementTimeout()
	batchSize := dbBatchSize()
	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}
		chunk := tasks[i:end]
		stmtCtx, cancel := context.WithTimeout(ctx, timeout)
		_, err := session.ExecuteWrite(stmtCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			for _, task := range chunk {
				res, err := tx.Run(stmtCtx, task.Statement, nil)
				if err != nil {
					return nil, fmt.Errorf("execute statement from %s: %w", task.File, err)
				}
				if _, err := res.Consume(stmtCtx); err != nil {
					return nil, fmt.Errorf("consume statement from %s: %w", task.File, err)
				}
				done++
				if progress != nil {
					progress(done, total, task.File)
				}
			}
			return nil, nil
		})
		cancel()
		if err != nil {
			if isTimeoutErr(err) && len(chunk) > 1 {
				for _, task := range chunk {
					stmtCtxSingle, cancelSingle := context.WithTimeout(ctx, timeout)
					_, serr := session.ExecuteWrite(stmtCtxSingle, func(tx neo4j.ManagedTransaction) (any, error) {
						res, err := tx.Run(stmtCtxSingle, task.Statement, nil)
						if err != nil {
							return nil, fmt.Errorf("execute statement from %s: %w", task.File, err)
						}
						_, err = res.Consume(stmtCtxSingle)
						if err != nil {
							return nil, fmt.Errorf("consume statement from %s: %w", task.File, err)
						}
						return nil, nil
					})
					cancelSingle()
					if serr != nil {
						if cfg.ContinueOnError {
							done++
							if progress != nil {
								progress(done, total, task.File)
							}
							continue
						}
						return done, serr
					}
					done++
					if progress != nil {
						progress(done, total, task.File)
					}
				}
				continue
			}
			if cfg.ContinueOnError {
				done += len(chunk)
				if progress != nil {
					progress(done, total, chunk[len(chunk)-1].File)
				}
				continue
			}
			return done, err
		}
	}
	return done, nil
}

func statementTimeout() time.Duration {
	v := strings.TrimSpace(os.Getenv("G2G_DB_STATEMENT_TIMEOUT_SEC"))
	if v == "" {
		return 120 * time.Second
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		return 120 * time.Second
	}
	return time.Duration(sec) * time.Second
}

func dbBatchSize() int {
	v := strings.TrimSpace(os.Getenv("G2G_DB_BATCH_SIZE"))
	if v == "" {
		return 100
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	v := strings.ToLower(err.Error())
	return strings.Contains(v, "context deadline exceeded") || strings.Contains(v, "timeout while reading")
}
