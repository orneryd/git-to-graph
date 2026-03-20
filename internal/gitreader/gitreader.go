package gitreader

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/c815719/git-to-graph/internal/model"
)

type Reader struct {
	repoPath string
}

func New(repoPath string) *Reader {
	return &Reader{repoPath: repoPath}
}

func (r *Reader) ValidateRepo() error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = r.repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("not a git repository: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *Reader) CommitList(from, to string) ([]string, error) {
	args := []string{"rev-list", "--reverse", "--topo-order"}
	rng := "HEAD"
	switch {
	case from != "" && to != "":
		rng = fmt.Sprintf("%s^..%s", from, to)
	case from != "":
		rng = fmt.Sprintf("%s^..HEAD", from)
	case to != "":
		rng = to
	}
	args = append(args, rng)

	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	var commits []string
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		v := strings.TrimSpace(s.Text())
		if v != "" {
			commits = append(commits, v)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, errors.New("no commits found for the selected range")
	}
	return commits, nil
}

func (r *Reader) CommitMeta(hash string) (model.Commit, error) {
	out, err := r.run("show", "-s", "--format=%H%x1f%an%x1f%ae%x1f%at%x1f%P%x1f%s", hash)
	if err != nil {
		return model.Commit{}, err
	}
	parts := strings.Split(strings.TrimSpace(out), "\x1f")
	if len(parts) < 6 {
		return model.Commit{}, fmt.Errorf("unexpected git show output for %s", hash)
	}
	epoch, err := parseUnix(parts[3])
	if err != nil {
		return model.Commit{}, err
	}
	parents := []string{}
	if strings.TrimSpace(parts[4]) != "" {
		parents = strings.Fields(parts[4])
	}
	return model.Commit{
		Hash:      parts[0],
		Author:    parts[1],
		Email:     parts[2],
		Timestamp: epoch,
		Parents:   parents,
		Message:   parts[5],
	}, nil
}

func (r *Reader) ChangedFiles(hash string) ([]model.FileChange, error) {
	out, err := r.run("show", "--format=", "--name-status", hash)
	if err != nil {
		return nil, err
	}
	var changes []model.FileChange
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		st := fields[0]
		switch {
		case strings.HasPrefix(st, "R") && len(fields) >= 3:
			changes = append(changes, model.FileChange{Status: "R", OldPath: fields[1], Path: fields[2]})
		case strings.HasPrefix(st, "D"):
			changes = append(changes, model.FileChange{Status: "D", Path: fields[1]})
		default:
			changes = append(changes, model.FileChange{Status: "M", Path: fields[1]})
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return changes, nil
}

func (r *Reader) FileAtCommit(hash, path string) (string, error) {
	out, err := r.run("show", fmt.Sprintf("%s:%s", hash, path))
	if err != nil {
		return "", err
	}
	return out, nil
}

func (r *Reader) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %s (%w)", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

func parseUnix(v string) (time.Time, error) {
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse commit time %q: %w", v, err)
	}
	return time.Unix(sec, 0).UTC(), nil
}
