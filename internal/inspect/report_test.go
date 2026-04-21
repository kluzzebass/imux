package inspect

import (
	"os"
	"testing"
	"time"
)

func TestBuildSelfProcess(t *testing.T) {
	t.Parallel()
	pid := os.Getpid()
	lines, next, _ := Build(pid, nil)
	if next == nil {
		t.Fatal("expected CPU sample")
	}
	if len(lines) < 2 {
		t.Fatalf("expected detail lines, got %#v", lines)
	}
	time.Sleep(50 * time.Millisecond)
	lines2, _, _ := Build(pid, next)
	if len(lines2) < 2 {
		t.Fatalf("expected second sample lines, got %#v", lines2)
	}
}
