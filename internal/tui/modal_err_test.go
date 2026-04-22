package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestModalSaveErrMessage(t *testing.T) {
	t.Parallel()
	got := modalSaveErrMessage(errors.New(`replace spec: process "uuid" still has an active child; stop it and wait for exit first`))
	if !strings.Contains(got, "Save still blocked") {
		t.Fatalf("expected friendly post-stop-blocked message, got %q", got)
	}
	other := modalSaveErrMessage(errors.New("something else broke"))
	if other != "something else broke" {
		t.Fatalf("passthrough: %q", other)
	}
}
