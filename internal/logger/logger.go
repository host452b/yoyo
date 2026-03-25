// internal/logger/logger.go
package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type Logger struct {
	ch        chan string
	done      chan struct{}
	closeOnce sync.Once
}

// New opens the log file and starts the background write goroutine.
// Caller must call Close() when done.
func New(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logger: open %s: %w", path, err)
	}
	l := &Logger{
		ch:   make(chan string, 256),
		done: make(chan struct{}),
	}
	go func() {
		defer f.Close()
		for msg := range l.ch {
			fmt.Fprint(f, msg)
		}
		close(l.done)
	}()
	return l, nil
}

// Infof writes an INFO log line. Never blocks — drops if buffer full.
func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", format, args...)
}

// Errorf writes an ERROR log line.
func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", format, args...)
}

func (l *Logger) log(level, format string, args ...any) {
	msg := fmt.Sprintf("[%s] %s %s\n",
		level,
		time.Now().Format("15:04:05.000"),
		fmt.Sprintf(format, args...),
	)
	select {
	case l.ch <- msg:
	default: // drop if buffer full to avoid blocking PTY loop
	}
}

// Close flushes all pending log entries and closes the file.
// Safe to call multiple times.
func (l *Logger) Close() {
	l.closeOnce.Do(func() {
		close(l.ch)
		<-l.done
	})
}
