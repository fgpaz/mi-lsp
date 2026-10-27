package processutil

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestCommandPreservesArgsAndEnvironment(t *testing.T) {
	cmd := Command("example-program", "first", "second")
	if !reflect.DeepEqual(cmd.Args, []string{"example-program", "first", "second"}) {
		t.Fatalf("args: %#v", cmd.Args)
	}
	cmd.Env = []string{"A=B"}
	if !reflect.DeepEqual(cmd.Env, []string{"A=B"}) {
		t.Fatalf("env: %#v", cmd.Env)
	}
}

func TestCommandContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Use an absolute existing executable so lookup failure cannot mask cancellation.
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := CommandContext(ctx, path)
	if err := cmd.Start(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start: %v", err)
	}
}
