package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type Reporter struct {
	out io.Writer
	mu  sync.Mutex

	phase   string
	detail  string
	current int
	total   int
	start   time.Time
}

func New(out io.Writer) *Reporter {
	return &Reporter{out: out, start: time.Now()}
}

func (r *Reporter) StartPhase(phase string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase = phase
	r.total = total
	r.current = 0
	r.detail = ""
	r.start = time.Now()
	r.flushLocked()
}

func (r *Reporter) Tick(detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current++
	r.detail = detail
	r.flushLocked()
}

func (r *Reporter) Progress(current int, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current < 0 {
		current = 0
	}
	r.current = current
	r.detail = detail
	r.flushLocked()
}

func (r *Reporter) Complete(summary string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := time.Since(r.start).Round(10 * time.Millisecond)
	fmt.Fprintf(r.out, "\r[%s] %d/%d done in %s - %s\n", r.phase, r.current, r.total, d, summary)
}

func (r *Reporter) Info(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, "%s\n", msg)
}

func (r *Reporter) flushLocked() {
	if r.total > 0 {
		fmt.Fprintf(r.out, "\r[%s] %d/%d %s", r.phase, r.current, r.total, r.detail)
		return
	}
	fmt.Fprintf(r.out, "\r[%s] %s", r.phase, r.detail)
}
