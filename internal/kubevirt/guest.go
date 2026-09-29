package kubevirt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func verifyGuest(ctx context.Context, settings *clients.Settings, namespace, vmiName string, config *runconfig.KubeVirtGuest) error {
	key, err := validateGuest(ctx, settings, namespace, config)
	if err != nil {
		return err
	}
	return runGuestCommand(ctx, settings, namespace, vmiName, config, key)
}

// ValidateGuest performs the read-only checks needed before a VMI is created.
func ValidateGuest(ctx context.Context, settings *clients.Settings, namespace string, config *runconfig.KubeVirtGuest) error {
	_, err := validateGuest(ctx, settings, namespace, config)
	return err
}

func validateGuest(ctx context.Context, settings *clients.Settings, namespace string, config *runconfig.KubeVirtGuest) ([]byte, error) {
	virtctl := config.Virtctl
	if virtctl == "" {
		virtctl = "virtctl"
	}
	if _, err := exec.LookPath(virtctl); err != nil {
		return nil, fmt.Errorf("virtctl is required for KubeVirt guest verification: %w", err)
	}
	secret, err := settings.K8s.CoreV1().Secrets(namespace).Get(ctx, config.PrivateKeySecret, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("guest private-key Secret %s/%s was not found", namespace, config.PrivateKeySecret)
		}
		return nil, fmt.Errorf("reading guest private-key Secret %s/%s: %w", namespace, config.PrivateKeySecret, err)
	}
	key, ok := secret.Data[config.PrivateKeyKey]
	if !ok || len(key) == 0 {
		return nil, fmt.Errorf("guest private-key Secret %s/%s has no non-empty %q key", namespace, config.PrivateKeySecret, config.PrivateKeyKey)
	}
	return key, nil
}

func runGuestCommand(ctx context.Context, settings *clients.Settings, namespace, vmiName string, config *runconfig.KubeVirtGuest, key []byte) error {
	virtctl := config.Virtctl
	if virtctl == "" {
		virtctl = "virtctl"
	}
	temporaryKey, err := os.CreateTemp("", "dra-harness-kubevirt-key-*")
	if err != nil {
		return fmt.Errorf("creating temporary guest key: %w", err)
	}
	keyPath := temporaryKey.Name()
	defer func() { _ = os.Remove(keyPath) }()
	if err := temporaryKey.Chmod(0600); err != nil {
		_ = temporaryKey.Close()
		return fmt.Errorf("protecting temporary guest key: %w", err)
	}
	if _, err := temporaryKey.Write(key); err != nil {
		_ = temporaryKey.Close()
		return fmt.Errorf("writing temporary guest key: %w", err)
	}
	if err := temporaryKey.Close(); err != nil {
		return fmt.Errorf("closing temporary guest key: %w", err)
	}

	target := "vmi/" + vmiName
	args := []string{
		"ssh",
		"--namespace", namespace,
		"--username", config.Username,
		"--identity-file", keyPath,
		"--local-ssh-opts", "-o StrictHostKeyChecking=no",
		"--command", config.Command,
		target,
	}
	command := exec.CommandContext(ctx, virtctl, args...)
	command.Env = os.Environ()
	if settings.Kubeconfig != "" {
		command.Env = append(command.Env, "KUBECONFIG="+settings.Kubeconfig)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("guest verification command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), config.ExpectedOutput) {
		return fmt.Errorf("guest verification output lacks %q: %s", config.ExpectedOutput, strings.TrimSpace(string(output)))
	}
	return nil
}
