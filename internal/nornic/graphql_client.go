package nornic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type GraphQLConfig struct {
	URL             string
	User            string
	Password        string
	Token           string
	Database        string
	TimeoutSeconds  int
	ContinueOnError bool
}

type GraphQLClient struct {
	cfg  GraphQLConfig
	http *http.Client
}

type ApplyProgressFunc func(done, total int, file string)

type statementTask struct {
	File      string
	Statement string
}

func NewGraphQLClient(cfg GraphQLConfig) *GraphQLClient {
	if cfg.URL == "" {
		cfg.URL = "http://localhost:7474/graphql"
	}
	if !strings.HasSuffix(cfg.URL, "/graphql") {
		cfg.URL = strings.TrimRight(cfg.URL, "/") + "/graphql"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	return &GraphQLClient{
		cfg:  cfg,
		http: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

type ExecuteResult struct {
	Rows int
}

type graphqlReq struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlResp struct {
	Data struct {
		ExecuteCypher struct {
			RowCount int `json:"rowCount"`
		} `json:"executeCypher"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

const executeMutation = `mutation ExecuteCypher($input: CypherInput!) {
  executeCypher(input: $input) {
    rowCount
  }
}`

func (c *GraphQLClient) ExecuteCypher(ctx context.Context, statement string, params map[string]any) (ExecuteResult, error) {
	input := map[string]any{"statement": statement}
	if len(params) > 0 {
		input["parameters"] = params
	}
	if c.cfg.Database != "" {
		input["database"] = c.cfg.Database
	}

	payload := graphqlReq{
		Query: executeMutation,
		Variables: map[string]any{
			"input": input,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ExecuteResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return ExecuteResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	} else if c.cfg.User != "" {
		req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ExecuteResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return ExecuteResult{}, fmt.Errorf("graphql status %d: %s", resp.StatusCode, string(raw))
	}

	var out graphqlResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return ExecuteResult{}, fmt.Errorf("decode graphql response: %w, body=%s", err, string(raw))
	}
	if len(out.Errors) > 0 {
		msgs := make([]string, 0, len(out.Errors))
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		return ExecuteResult{}, fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	return ExecuteResult{Rows: out.Data.ExecuteCypher.RowCount}, nil
}

func ApplyCypherFiles(ctx context.Context, client *GraphQLClient, paths []string, progress ApplyProgressFunc) (int, error) {
	tasks, err := buildStatementTasks(paths)
	if err != nil {
		return 0, err
	}
	total := len(tasks)
	done := 0
	for _, task := range tasks {
		if _, err := client.ExecuteCypher(ctx, task.Statement, nil); err != nil {
			if client.cfg.ContinueOnError {
				done++
				if progress != nil {
					progress(done, total, task.File)
				}
				continue
			}
			return done, fmt.Errorf("execute statement from %s: %w", task.File, err)
		}
		done++
		if progress != nil {
			progress(done, total, task.File)
		}
	}
	return done, nil
}

func buildStatementTasks(paths []string) ([]statementTask, error) {
	tasks := make([]statementTask, 0, 256)
	for _, p := range paths {
		buf, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		stmts := splitCypherStatements(string(buf))
		for _, stmt := range stmts {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			tasks = append(tasks, statementTask{File: p, Statement: stmt})
		}
	}
	return tasks, nil
}

func CountStatements(paths []string) (int, error) {
	tasks, err := buildStatementTasks(paths)
	if err != nil {
		return 0, err
	}
	return len(tasks), nil
}

func splitCypherStatements(v string) []string {
	v = stripCypherComments(v)
	var out []string
	var cur strings.Builder
	inSingle := false
	esc := false
	for _, r := range v {
		cur.WriteRune(r)
		if esc {
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		if r == '\'' {
			inSingle = !inSingle
			continue
		}
		if r == ';' && !inSingle {
			stmt := strings.TrimSpace(cur.String())
			stmt = strings.TrimSuffix(stmt, ";")
			if stmt != "" {
				out = append(out, stmt)
			}
			cur.Reset()
		}
	}
	if tail := strings.TrimSpace(cur.String()); tail != "" {
		out = append(out, tail)
	}
	return out
}

func stripCypherComments(v string) string {
	lines := strings.Split(v, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "//") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}
