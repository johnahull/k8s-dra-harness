package harness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type release struct{ namespace, name string }

// releaseManager keeps install intent and cleanup decisions in one place.
// Intent is recorded before Helm runs because a failed install can leave a
// release behind. Cleanup only uninstalls when Helm confirms it is present.
type releaseManager struct {
	owned     []release
	lookup    func(context.Context, release) (bool, error)
	install   func(context.Context, release, string, map[string]any) error
	uninstall func(context.Context, release) error
}

func (m *releaseManager) exists(ctx context.Context, rel release) (bool, error) {
	if m.lookup != nil {
		return m.lookup(ctx, rel)
	}
	return releaseExists(ctx, rel)
}

func (m *releaseManager) requireAbsent(ctx context.Context, rel release) error {
	exists, err := m.exists(ctx, rel)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("helm release %s/%s already exists", rel.namespace, rel.name)
	}
	return nil
}

func (m *releaseManager) installRelease(ctx context.Context, rel release, chart string, values map[string]any) error {
	m.owned = append(m.owned, rel)
	if m.install != nil {
		return m.install(ctx, rel, chart, values)
	}
	return helmInstall(ctx, rel.name, chart, rel.namespace, values)
}

func (m *releaseManager) cleanup(ctx context.Context) error {
	var problems []error
	for i := len(m.owned) - 1; i >= 0; i-- {
		rel := m.owned[i]
		exists, err := m.exists(ctx, rel)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if !exists {
			continue
		}
		if m.uninstall != nil {
			err = m.uninstall(ctx, rel)
		} else {
			err = run(ctx, "helm", "uninstall", rel.name, "--namespace", rel.namespace, "--wait", "--timeout", "10m")
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("uninstalling Helm release %s/%s: %w", rel.namespace, rel.name, err))
		}
	}
	return errors.Join(problems...)
}

func releaseExists(ctx context.Context, rel release) (bool, error) {
	// Helm 4 lists releases in every state by default and removed the
	// deprecated --all flag. The same command is also valid on Helm 3.
	cmd := exec.CommandContext(ctx, "helm", "list", "--namespace", rel.namespace, "--filter", "^"+rel.name+"$", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking Helm release %s/%s: %w: %s", rel.namespace, rel.name, err, out)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
