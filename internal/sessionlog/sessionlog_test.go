package sessionlog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tee := filepath.Join(dir, "tee.log")
	s, err := Open(tee)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Append(Record{K: KindStdout, ID: "a", Name: "one", Msg: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Record{K: KindStderr, ID: "b", Name: "two", Msg: "warn"}); err != nil {
		t.Fatal(err)
	}

	n, err := s.LineCount()
	if err != nil || n != 2 {
		t.Fatalf("line count = %d %v", n, err)
	}
	r0, err := s.ReadLine(0)
	if err != nil {
		t.Fatal(err)
	}
	if r0.K != KindStdout || r0.Msg != "hello" || r0.Name != "one" {
		t.Fatalf("r0 %#v", r0)
	}
	r1, err := s.ReadLine(1)
	if err != nil {
		t.Fatal(err)
	}
	if r1.K != KindStderr || r1.Msg != "warn" {
		t.Fatalf("r1 %#v", r1)
	}
}

func TestSessionLogNoTee(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Append(Record{K: KindStdout, ID: "x", Msg: "x", T: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
	n, _ := s.LineCount()
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
}
