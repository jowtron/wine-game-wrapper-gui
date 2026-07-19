package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ProgressReporter is the interface that pipeline functions use to report progress.
type ProgressReporter interface {
	Log(msg string)
	Logf(format string, args ...interface{})
	Step(step, total int, name string)
	Error(msg string)
	Writer() io.Writer
}

// WailsReporter emits Wails events for progress reporting.
type WailsReporter struct {
	ctx context.Context
}

func NewWailsReporter(ctx context.Context) *WailsReporter {
	return &WailsReporter{ctx: ctx}
}

func (r *WailsReporter) Log(msg string) {
	runtime.EventsEmit(r.ctx, "build:log", msg)
}

func (r *WailsReporter) Logf(format string, args ...interface{}) {
	r.Log(fmt.Sprintf(format, args...))
}

func (r *WailsReporter) Step(step, total int, name string) {
	runtime.EventsEmit(r.ctx, "build:step", map[string]interface{}{
		"step":  step,
		"total": total,
		"name":  name,
	})
	r.Logf("[%d/%d] %s", step, total, name)
}

func (r *WailsReporter) Error(msg string) {
	runtime.EventsEmit(r.ctx, "build:error", msg)
}

func (r *WailsReporter) Writer() io.Writer {
	return &eventWriter{r: r}
}

// CLIReporter prints progress to stdout for headless builds.
type CLIReporter struct{}

func NewCLIReporter() *CLIReporter { return &CLIReporter{} }

func (r *CLIReporter) Log(msg string) { fmt.Println(msg) }

func (r *CLIReporter) Logf(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

func (r *CLIReporter) Step(step, total int, name string) {
	fmt.Printf("[%d/%d] %s\n", step, total, name)
}

func (r *CLIReporter) Error(msg string) { fmt.Fprintln(os.Stderr, msg) }

func (r *CLIReporter) Writer() io.Writer { return &eventWriter{r: r} }

// eventWriter splits subprocess output into lines and emits each as a log event.
type eventWriter struct {
	r   ProgressReporter
	buf bytes.Buffer
	mu  sync.Mutex
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Put back the incomplete line
			w.buf.WriteString(line)
			break
		}
		// Trim trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if line != "" {
			w.r.Log("  " + line)
		}
	}
	return len(p), nil
}
