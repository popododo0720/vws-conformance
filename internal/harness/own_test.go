package harness

import (
	"context"
	"testing"
)

func TestOwnRegistersDeletion(t *testing.T) {
	rt := NewRuntime(Config{RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir()}, nil)
	hit := false
	if err := rt.Own("subnet", "subnet-1", "conf-sub", func(context.Context) error {
		hit = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil || !hit {
		t.Fatalf("err %v hit %v", err, hit)
	}
}
