package nornic

import (
	"context"
	"fmt"
	"os"
	"strings"

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

func ApplyCypherFilesBolt(ctx context.Context, cfg BoltConfig, paths []string) (int, error) {
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

	total := 0
	for _, p := range paths {
		buf, err := os.ReadFile(p)
		if err != nil {
			return total, err
		}
		stmts := splitCypherStatements(string(buf))
		for _, stmt := range stmts {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
				res, err := tx.Run(ctx, stmt, nil)
				if err != nil {
					return nil, err
				}
				_, err = res.Consume(ctx)
				return nil, err
			})
			if err != nil {
				if cfg.ContinueOnError {
					total++
					continue
				}
				return total, fmt.Errorf("execute statement from %s: %w", p, err)
			}
			total++
		}
	}
	return total, nil
}
