package cli

import (
	"testing"

	"imux/internal/tui"
)

func TestParseTUIModeArgsBootstrap(t *testing.T) {
	t.Parallel()
	opts, err := ParseTUIModeArgs([]string{"--name", "a,b", "--tee", "/tmp/x", "echo hi", "echo lo"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.TeePath != "/tmp/x" {
		t.Fatalf("tee %q", opts.TeePath)
	}
	if len(opts.Bootstrap) != 2 {
		t.Fatalf("bootstrap len %d", len(opts.Bootstrap))
	}
	if opts.Bootstrap[0] != (tui.BootstrapProc{ID: "a", Line: "echo hi"}) {
		t.Fatalf("boot0 %+v", opts.Bootstrap[0])
	}
	if opts.Bootstrap[1].Line != "echo lo" {
		t.Fatalf("boot1 %+v", opts.Bootstrap[1])
	}
}

func TestParseTUIModeArgsFlagsOnly(t *testing.T) {
	t.Parallel()
	opts, err := ParseTUIModeArgs([]string{"--tee", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.TeePath != "x" || len(opts.Bootstrap) != 0 {
		t.Fatalf("%+v", opts)
	}
}

func TestParseTUIModeArgsNameWithoutCommands(t *testing.T) {
	t.Parallel()
	_, err := ParseTUIModeArgs([]string{"--name", "a,b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTUIModeArgsDuplicateSlotNames(t *testing.T) {
	t.Parallel()
	_, err := ParseTUIModeArgs([]string{"--name", "ps1,ps1", "true", "true"})
	if err == nil {
		t.Fatal("expected error")
	}
}
