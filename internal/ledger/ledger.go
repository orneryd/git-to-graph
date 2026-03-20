package ledger

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/c815719/git-to-graph/internal/model"
)

type CodeState struct {
	CodeKey    string     `json:"code_key"`
	ValueJSON  string     `json:"value_json"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidTo    *time.Time `json:"valid_to,omitempty"`
	AssertedAt time.Time  `json:"asserted_at"`
	AssertedBy string     `json:"asserted_by"`
	CommitHash string     `json:"commit_hash"`
	TxID       string     `json:"tx_id"`
}

type CodeChange struct {
	ChangeID        string    `json:"change_id"`
	TxID            string    `json:"tx_id"`
	Actor           string    `json:"actor"`
	Timestamp       time.Time `json:"timestamp"`
	OpType          string    `json:"op_type"`
	CommitHash      string    `json:"commit_hash"`
	AffectedCode    string    `json:"affected_code"`
	AffectedStateID string    `json:"affected_state_id,omitempty"`
}

type FactVersion = CodeState
type MutationEvent = CodeChange

type ApplyStats struct {
	Created int
	Closed  int
	Kept    int
}

type Ledger struct {
	states  []CodeState
	changes []CodeChange
	current map[string]*CodeState
	txSeq   int
}

func New() *Ledger {
	return &Ledger{current: map[string]*CodeState{}}
}

func (l *Ledger) ApplySnapshot(facts map[string]string, commit model.Commit) ApplyStats {
	stats := ApplyStats{}
	l.txSeq++
	txID := fmt.Sprintf("tx-%s-%06d", shortHash(commit.Hash), l.txSeq)

	for key, prev := range l.current {
		_, exists := facts[key]
		if exists {
			continue
		}
		ts := commit.Timestamp
		prev.ValidTo = &ts
		stats.Closed++
		l.changes = append(l.changes, CodeChange{
			ChangeID:        fmt.Sprintf("change-%s-close-%s", shortHash(commit.Hash), sanitizeKey(key)),
			TxID:            txID,
			Actor:           commit.Author,
			Timestamp:       commit.Timestamp,
			OpType:          "CLOSE_CODE_STATE",
			CommitHash:      commit.Hash,
			AffectedCode:    key,
			AffectedStateID: prev.StateID(),
		})
		delete(l.current, key)
	}

	for key, value := range facts {
		if prev, ok := l.current[key]; ok {
			if prev.ValueJSON == value {
				stats.Kept++
				continue
			}
			ts := commit.Timestamp
			prev.ValidTo = &ts
			stats.Closed++
		}

		cs := CodeState{
			CodeKey:    key,
			ValueJSON:  value,
			ValidFrom:  commit.Timestamp,
			AssertedAt: commit.Timestamp,
			AssertedBy: commit.Author,
			CommitHash: commit.Hash,
			TxID:       txID,
		}
		l.states = append(l.states, cs)
		l.current[key] = &l.states[len(l.states)-1]
		stats.Created++
		l.changes = append(l.changes, CodeChange{
			ChangeID:        fmt.Sprintf("change-%s-upsert-%s", shortHash(commit.Hash), sanitizeKey(key)),
			TxID:            txID,
			Actor:           commit.Author,
			Timestamp:       commit.Timestamp,
			OpType:          "UPSERT_CODE_STATE",
			CommitHash:      commit.Hash,
			AffectedCode:    key,
			AffectedStateID: cs.StateID(),
		})
	}

	return stats
}

func (l *Ledger) CodeStates() []CodeState {
	return l.states
}

func (l *Ledger) CodeChanges() []CodeChange {
	return l.changes
}

func (l *Ledger) Versions() []CodeState {
	return l.states
}

func (l *Ledger) Events() []CodeChange {
	return l.changes
}

func WriteJSONL[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func LoadSnapshotAt(ledgerPath string, at time.Time) (map[string]CodeState, error) {
	f, err := os.Open(ledgerPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	snap := map[string]CodeState{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		var cs CodeState
		if err := json.Unmarshal(line, &cs); err != nil {
			return nil, err
		}
		if !cs.ValidFrom.After(at) && (cs.ValidTo == nil || cs.ValidTo.After(at)) {
			snap[cs.CodeKey] = cs
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return snap, nil
}

func WriteSnapshot(w io.Writer, snapshot map[string]CodeState) error {
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	enc := json.NewEncoder(w)
	for _, k := range keys {
		if err := enc.Encode(snapshot[k]); err != nil {
			return err
		}
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func sanitizeKey(v string) string {
	out := make([]rune, 0, len(v))
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "key"
	}
	if len(out) > 24 {
		return string(out[:24])
	}
	return string(out)
}

func (s CodeState) StateID() string {
	return codeStateID(s.CodeKey, s.TxID, s.CommitHash, s.ValidFrom)
}

func codeStateID(codeKey, txID, commitHash string, validFrom time.Time) string {
	validFromISO := validFrom.UTC().Format("2006-01-02T15:04:05Z")
	payload := fmt.Sprintf("%s|%s|%s|%s", codeKey, txID, commitHash, validFromISO)
	sum := sha1.Sum([]byte(payload))
	return "cs-" + hex.EncodeToString(sum[:])
}
