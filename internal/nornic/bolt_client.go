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
	nodesCreated := 0
	relationshipsCreated := 0

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
		resAny, err := session.ExecuteWrite(stmtCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(stmtCtx, qVersions, map[string]any{"rows": rows})
			if err != nil {
				return nil, err
			}
			summary, err := res.Consume(stmtCtx)
			if err != nil {
				return nil, err
			}
			n, r := summaryCreatedCounts(summary)
			return [2]int{n, r}, nil
		})
		cancel()
		if err != nil {
			// Fallback: some Nornic builds reject this UNWIND mutation shape.
			logFallbackQueryError("versions", err, qVersions, qVersionRow)
			chunkCtx, chunkCancel := context.WithTimeout(ctx, timeout)
			chunkResAny, chunkErr := session.ExecuteWrite(chunkCtx, func(tx neo4j.ManagedTransaction) (any, error) {
				localNodes := 0
				localEdges := 0
				for j, row := range rows {
					res, runErr := tx.Run(chunkCtx, qVersionRow, row)
					if runErr != nil {
						return nil, fmt.Errorf("row %d run: %w", i+j, runErr)
					}
					summary, consumeErr := res.Consume(chunkCtx)
					if consumeErr != nil {
						return nil, fmt.Errorf("row %d consume: %w", i+j, consumeErr)
					}
					n, r := summaryCreatedCounts(summary)
					localNodes += n
					localEdges += r
				}
				return [2]int{localNodes, localEdges}, nil
			})
			chunkCancel()
			if chunkErr != nil {
				return done, fmt.Errorf("apply version batch [%d:%d] (row fallback txn failed): %w", i, end, chunkErr)
			}
			if counts, ok := chunkResAny.([2]int); ok {
				nodesCreated += counts[0]
				relationshipsCreated += counts[1]
			}
			done += len(rows)
			if progress != nil {
				progress(done, total, fmt.Sprintf("nodes=%d edges=%d", nodesCreated, relationshipsCreated))
			}
			continue
		}
		if counts, ok := resAny.([2]int); ok {
			nodesCreated += counts[0]
			relationshipsCreated += counts[1]
		}
		done += len(rows)
		if progress != nil {
			progress(done, total, fmt.Sprintf("nodes=%d edges=%d", nodesCreated, relationshipsCreated))
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
OPTIONAL MATCH (csByID:CodeState {state_id: row.affected_state_id})
OPTIONAL MATCH (csByKey:CodeState {code_key: row.affected_code_key, tx_id: row.tx_id})
WITH cc, coalesce(csByID, csByKey) AS cs
WHERE cs IS NOT NULL
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
OPTIONAL MATCH (csByID:CodeState {state_id: $affected_state_id})
OPTIONAL MATCH (csByKey:CodeState {code_key: $affected_code_key, tx_id: $tx_id})
WITH cc, coalesce(csByID, csByKey) AS cs
WHERE cs IS NOT NULL
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
				"affected_code_key": ev.AffectedCode,
			})
		}
		stmtCtx, cancel := context.WithTimeout(ctx, timeout)
		resAny, err := session.ExecuteWrite(stmtCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(stmtCtx, qEvents, map[string]any{"rows": rows})
			if err != nil {
				return nil, err
			}
			summary, err := res.Consume(stmtCtx)
			if err != nil {
				return nil, err
			}
			n, r := summaryCreatedCounts(summary)
			return [2]int{n, r}, nil
		})
		cancel()
		if err != nil {
			// Fallback: execute event upserts row-by-row when UNWIND mutation is rejected.
			logFallbackQueryError("events", err, qEvents, qEventRow)
			chunkCtx, chunkCancel := context.WithTimeout(ctx, timeout)
			chunkResAny, chunkErr := session.ExecuteWrite(chunkCtx, func(tx neo4j.ManagedTransaction) (any, error) {
				localNodes := 0
				localEdges := 0
				for j, row := range rows {
					res, runErr := tx.Run(chunkCtx, qEventRow, row)
					if runErr != nil {
						return nil, fmt.Errorf("row %d run: %w", i+j, runErr)
					}
					summary, consumeErr := res.Consume(chunkCtx)
					if consumeErr != nil {
						return nil, fmt.Errorf("row %d consume: %w", i+j, consumeErr)
					}
					n, r := summaryCreatedCounts(summary)
					localNodes += n
					localEdges += r
				}
				return [2]int{localNodes, localEdges}, nil
			})
			chunkCancel()
			if chunkErr != nil {
				return done, fmt.Errorf("apply event batch [%d:%d] (row fallback txn failed): %w", i, end, chunkErr)
			}
			if counts, ok := chunkResAny.([2]int); ok {
				nodesCreated += counts[0]
				relationshipsCreated += counts[1]
			}
			done += len(rows)
			if progress != nil {
				progress(done, total, fmt.Sprintf("nodes=%d edges=%d", nodesCreated, relationshipsCreated))
			}
			continue
		}
		if counts, ok := resAny.([2]int); ok {
			nodesCreated += counts[0]
			relationshipsCreated += counts[1]
		}
		done += len(rows)
		if progress != nil {
			progress(done, total, fmt.Sprintf("nodes=%d edges=%d", nodesCreated, relationshipsCreated))
		}
	}

	if len(events) > 0 {
		checkCtx, checkCancel := context.WithTimeout(ctx, timeout)
		defer checkCancel()
		res, err := session.ExecuteRead(checkCtx, func(tx neo4j.ManagedTransaction) (any, error) {
			r, runErr := tx.Run(checkCtx, "MATCH (:CodeChange)-[:IMPACTS]->(:CodeState) RETURN count(*) AS c", nil)
			if runErr != nil {
				return nil, runErr
			}
			if !r.Next(checkCtx) {
				if r.Err() != nil {
					return nil, r.Err()
				}
				return int64(0), nil
			}
			v, _ := r.Record().Get("c")
			if c, ok := v.(int64); ok {
				return c, nil
			}
			return int64(0), nil
		})
		if err != nil {
			return done, err
		}
		if c, _ := res.(int64); c == 0 {
			return done, fmt.Errorf("apply validation failed: no CodeChange-[:IMPACTS]->CodeState edges were created")
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

func summaryCreatedCounts(summary neo4j.ResultSummary) (int, int) {
	if summary == nil {
		return 0, 0
	}
	counters := summary.Counters()
	if counters == nil {
		return 0, 0
	}
	return counters.NodesCreated(), counters.RelationshipsCreated()
}

func logFallbackQueryError(stage string, runErr error, failedQuery, fallbackQuery string) {
	_, _ = fmt.Fprintf(os.Stderr, "\n[g2g][apply_nornic][fallback] stage=%s reason=%v\n", stage, runErr)
	_, _ = fmt.Fprintf(os.Stderr, "[g2g][apply_nornic][fallback] failed_unwind_query:\n%s\n", failedQuery)
	_, _ = fmt.Fprintf(os.Stderr, "[g2g][apply_nornic][fallback] fallback_row_query:\n%s\n", fallbackQuery)
}
