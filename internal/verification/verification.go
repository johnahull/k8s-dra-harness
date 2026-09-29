// Package verification runs the topology repository's read-only verification
// scripts and records their output as run evidence.
package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Config controls the external verification checkout and evidence directory.
type Config struct {
	ScriptsDir         string
	ExpectedRepoCommit string
	EvidenceDir        string
	Commands           [][]string
}

// Runner executes configured verification commands. It never invokes a shell.
type Runner struct {
	config     Config
	kubeconfig string
}

// New validates the configured script checkout and returns a runner. An empty
// ScriptsDir disables external verification while preserving API snapshots.
func New(config Config, kubeconfig string) (*Runner, error) {
	if config.ScriptsDir == "" {
		if config.EvidenceDir != "" {
			if err := os.MkdirAll(config.EvidenceDir, 0750); err != nil {
				return nil, fmt.Errorf("creating verification evidence directory: %w", err)
			}
		}
		return &Runner{config: config, kubeconfig: kubeconfig}, nil
	}
	info, err := os.Stat(config.ScriptsDir)
	if err != nil {
		return nil, fmt.Errorf("checking verification scripts directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("verification scripts path %q is not a directory", config.ScriptsDir)
	}
	if config.EvidenceDir == "" {
		return nil, fmt.Errorf("verification scripts require evidenceDir")
	}
	if err := os.MkdirAll(config.EvidenceDir, 0750); err != nil {
		return nil, fmt.Errorf("creating verification evidence directory: %w", err)
	}
	commit, err := gitCommit(config.ScriptsDir)
	if err != nil {
		return nil, err
	}
	if config.ExpectedRepoCommit != "" && commit != config.ExpectedRepoCommit {
		return nil, fmt.Errorf("verification scripts checkout is %s, want %s", commit, config.ExpectedRepoCommit)
	}
	if config.ExpectedRepoCommit != "" {
		if dirty, err := gitDirty(config.ScriptsDir); err != nil {
			return nil, err
		} else if dirty {
			return nil, fmt.Errorf("verification scripts checkout %s has uncommitted changes", config.ScriptsDir)
		}
	}
	if err := os.WriteFile(filepath.Join(config.EvidenceDir, "verification-repo-commit.txt"), []byte(commit+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("recording verification repository commit: %w", err)
	}
	return &Runner{config: config, kubeconfig: kubeconfig}, nil
}

func gitCommit(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading verification repository commit: %w", err)
	}
	commit := strings.TrimSpace(string(output))
	if commit == "" {
		return "", fmt.Errorf("verification repository returned an empty commit")
	}
	return commit, nil
}

func gitDirty(dir string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking verification repository status: %w", err)
	}
	return len(output) != 0, nil
}

// Enabled reports whether external scripts are configured.
func (r *Runner) Enabled() bool { return r != nil && r.config.ScriptsDir != "" }

// Run executes one configured script command and saves stdout and stderr.
func (r *Runner) Run(ctx context.Context, name, namespace string, args ...string) error {
	if !r.Enabled() {
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("verification command %q has no executable", name)
	}
	command, commandArgs, err := r.resolve(args)
	if err != nil {
		return err
	}
	base := filepath.Join(r.config.EvidenceDir, sanitize(name))
	stdoutPath, stderrPath := base+".stdout", base+".stderr"
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("creating verification stdout: %w", err)
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("creating verification stderr: %w", err)
	}
	defer func() { _ = stderr.Close() }()

	cmd := exec.CommandContext(ctx, command, commandArgs...)
	cmd.Env = append(os.Environ(), "DRA_HARNESS_NAMESPACE="+namespace)
	if r.kubeconfig != "" {
		cmd.Env = append(cmd.Env, "KUBECONFIG="+r.kubeconfig)
	}
	cmd.Stdout = io.MultiWriter(os.Stdout, stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("verification command %q failed: %w", name, err)
	}
	return nil
}

// RunConfigured executes every command listed in the run configuration.
func (r *Runner) RunConfigured(ctx context.Context, namespace, prefix string) error {
	for i, command := range r.config.Commands {
		name := fmt.Sprintf("%s-configured-%02d", sanitize(prefix), i)
		if len(command) > 0 {
			name = fmt.Sprintf("%s-configured-%02d-%s", sanitize(prefix), i, filepath.Base(command[0]))
		}
		if err := r.Run(ctx, name, namespace, command...); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot records the names of API JSON files written for one phase.
type Snapshot struct {
	Prefix         string
	ResourceSlices string
	ResourceClaims string
}

// SaveSnapshot writes all ResourceClaims and cluster-scoped ResourceSlices in
// the shape consumed by dra-counters.py. The counter verifier needs claims
// from every namespace to correlate allocations with shared devices.
func (r *Runner) SaveSnapshot(ctx context.Context, client kubernetes.Interface, prefix string) (Snapshot, error) {
	if r == nil || r.config.EvidenceDir == "" {
		return Snapshot{Prefix: prefix}, nil
	}
	slices, err := client.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("listing ResourceSlices for snapshot: %w", err)
	}
	claims, err := client.ResourceV1().ResourceClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("listing ResourceClaims for snapshot: %w", err)
	}
	snapshot := Snapshot{
		Prefix:         prefix,
		ResourceSlices: filepath.Join(r.config.EvidenceDir, sanitize(prefix)+"-resourceslices.json"),
		ResourceClaims: filepath.Join(r.config.EvidenceDir, sanitize(prefix)+"-resourceclaims.json"),
	}
	if err := writeJSON(snapshot.ResourceSlices, slices); err != nil {
		return Snapshot{}, err
	}
	if err := writeJSON(snapshot.ResourceClaims, claims); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding snapshot %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing snapshot %s: %w", path, err)
	}
	return nil
}

// RunCounterReport runs dra-counters.py against a saved snapshot pair.
func (r *Runner) RunCounterReport(ctx context.Context, namespace string, snapshot Snapshot) error {
	if !r.Enabled() {
		return nil
	}
	return r.Run(ctx, snapshot.Prefix+"-counters", namespace, "dra-counters.py", snapshot.ResourceSlices, snapshot.ResourceClaims)
}

func (r *Runner) resolve(args []string) (string, []string, error) {
	name := filepath.Base(args[0])
	script := args[0]
	if !filepath.IsAbs(script) {
		script = filepath.Join(r.config.ScriptsDir, script)
	}
	root, err := filepath.Abs(r.config.ScriptsDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolving verification scripts directory: %w", err)
	}
	script, err = filepath.Abs(script)
	if err != nil {
		return "", nil, fmt.Errorf("resolving verification command %q: %w", args[0], err)
	}
	rel, err := filepath.Rel(root, script)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("verification command %q is outside scriptsDir", args[0])
	}
	if _, err := os.Stat(script); err != nil {
		return "", nil, fmt.Errorf("checking verification command %q: %w", script, err)
	}
	if strings.HasSuffix(name, ".py") {
		return "python3", append([]string{script}, args[1:]...), nil
	}
	return script, args[1:], nil
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "verification-" + time.Now().UTC().Format("20060102-150405")
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}
