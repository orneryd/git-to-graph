package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/c815719/git-to-graph/internal/model"
)

type FactVersion struct {
	FactKey    string     `json:"fact_key"`
	ValueJSON  string     `json:"value_json"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidTo    *time.Time `json:"valid_to,omitempty"`
	AssertedAt time.Time  `json:"asserted_at"`
	AssertedBy string     `json:"asserted_by"`
	CommitHash string     `json:"commit_hash"`
	TxID       string     `json:"tx_id"`
}

type MutationEvent struct {
	EventID      string    `json:"event_id"`
	TxID         string    `json:"tx_id"`
	Actor        string    `json:"actor"`
	Timestamp    time.Time `json:"timestamp"`
	OpType       string    `json:"op_type"`
	CommitHash   string    `json:"commit_hash"`
	AffectedFact string    `json:"affected_fact"`
}

type ApplyStats struct {
	Created int
	Closed  int
	Kept    int
}

type Ledger struct {
	versions []FactVersion
	events   []MutationEvent
	current  map[string]*FactVersion
	txSeq    int
}

func New() *Ledger {
	return &Ledger{current: map[string]*FactVersion{}}
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
		l.events = append(l.events, MutationEvent{
			EventID:      fmt.Sprintf("event-%s-close-%s", shortHash(commit.Hash), sanitizeKey(key)),
			TxID:         txID,
			Actor:        commit.Author,
			Timestamp:    commit.Timestamp,
			OpType:       "CLOSE_FACT_VERSION",
			CommitHash:   commit.Hash,
			AffectedFact: key,
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

		fv := FactVersion{
			FactKey:    key,
			ValueJSON:  value,
			ValidFrom:  commit.Timestamp,
			AssertedAt: commit.Timestamp,
			AssertedBy: commit.Author,
			CommitHash: commit.Hash,
			TxID:       txID,
		}
		l.versions = append(l.versions, fv)
		l.current[key] = &l.versions[len(l.versions)-1]
		stats.Created++
		l.events = append(l.events, MutationEvent{
			EventID:      fmt.Sprintf("event-%s-upsert-%s", shortHash(commit.Hash), sanitizeKey(key)),
			TxID:         txID,
			Actor:        commit.Author,
			Timestamp:    commit.Timestamp,
			OpType:       "UPSERT_FACT_VERSION",
			CommitHash:   commit.Hash,
			AffectedFact: key,
		})
	}

	return stats
}

func (l *Ledger) Versions() []FactVersion {
	return l.versions
}

func (l *Ledger) Events() []MutationEvent {
	return l.events
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

func LoadSnapshotAt(ledgerPath string, at time.Time) (map[string]FactVersion, error) {
	f, err := os.Open(ledgerPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	snap := map[string]FactVersion{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		var fv FactVersion
		if err := json.Unmarshal(line, &fv); err != nil {
			return nil, err
		}
		if !fv.ValidFrom.After(at) && (fv.ValidTo == nil || fv.ValidTo.After(at)) {
			snap[fv.FactKey] = fv
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return snap, nil
}

func WriteSnapshot(w io.Writer, snapshot map[string]FactVersion) error {
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
