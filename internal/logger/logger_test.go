// internal/logger/logger_test.go
package logger_test

import (
	"os"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/logger"
)

func TestLogger_WritesToFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "yoyo-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	log, err := logger.New(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.Infof("hello %s", "world")
	log.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("expected log file to contain 'hello world', got: %s", data)
	}
}

func TestLogger_Async(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.log")
	f.Close()
	log, _ := logger.New(f.Name())

	for i := 0; i < 100; i++ {
		log.Infof("line %d", i)
	}
	log.Close() // flush before read

	data, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(data), "line 99") {
		t.Error("expected all 100 lines to be flushed")
	}
}
