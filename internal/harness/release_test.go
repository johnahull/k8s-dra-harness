package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReleaseManagerCleanupPreservesStateOnLookupFailure(t *testing.T) {
	ctx := context.Background()
	rel := release{namespace: "test", name: "driver"}
	uninstalled := false
	m := releaseManager{
		owned:  []release{rel},
		lookup: func(context.Context, release) (bool, error) { return false, errors.New("helm unavailable") },
		uninstall: func(context.Context, release) error {
			uninstalled = true
			return nil
		},
	}
	if err := m.cleanup(ctx); err == nil || !strings.Contains(err.Error(), "helm unavailable") {
		t.Fatalf("cleanup error = %v", err)
	}
	if uninstalled {
		t.Fatal("cleanup uninstalled a release whose state is unknown")
	}
}

func TestReleaseManagerTracksFailedInstallForCleanup(t *testing.T) {
	ctx := context.Background()
	rel := release{namespace: "test", name: "driver"}
	uninstalled := false
	m := releaseManager{
		lookup:  func(context.Context, release) (bool, error) { return true, nil },
		install: func(context.Context, release, string, map[string]any) error { return errors.New("timed out") },
		uninstall: func(context.Context, release) error {
			uninstalled = true
			return nil
		},
	}
	if err := m.installRelease(ctx, rel, "chart", nil); err == nil {
		t.Fatal("expected install failure")
	}
	if err := m.cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if !uninstalled {
		t.Fatal("cleanup did not uninstall the partial release")
	}
}

func TestReleaseManagerSkipsAbsentRelease(t *testing.T) {
	uninstalled := false
	m := releaseManager{
		owned:  []release{{namespace: "test", name: "driver"}},
		lookup: func(context.Context, release) (bool, error) { return false, nil },
		uninstall: func(context.Context, release) error {
			uninstalled = true
			return nil
		},
	}
	if err := m.cleanup(context.Background()); err != nil || uninstalled {
		t.Fatalf("cleanup = %v, uninstalled = %t", err, uninstalled)
	}
}
