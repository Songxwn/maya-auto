package logbuf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Line struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

type Buffer struct {
	mu    sync.Mutex
	lines []Line
	limit int
	file  *os.File
}

func New(path string, limit int) *Buffer {
	if limit <= 0 {
		limit = 400
	}
	b := &Buffer{limit: limit}
	if path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			b.file = f
		}
	}
	return b
}

func (b *Buffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.file != nil {
		_ = b.file.Close()
		b.file = nil
	}
}

func (b *Buffer) Info(text string)  { b.add("info", text) }
func (b *Buffer) Error(text string) { b.add("error", text) }

func (b *Buffer) add(level, text string) {
	line := Line{
		Time:  time.Now().Format("15:04:05"),
		Level: level,
		Text:  text,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.limit {
		b.lines = b.lines[len(b.lines)-b.limit:]
	}
	if b.file != nil {
		_, _ = fmt.Fprintf(b.file, "%s %s %s\n", line.Time, level, text)
	}
}

func (b *Buffer) Snapshot() []Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Line, len(b.lines))
	copy(out, b.lines)
	return out
}
