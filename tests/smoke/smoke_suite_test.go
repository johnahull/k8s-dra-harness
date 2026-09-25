//go:build integration

package smoke

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(Fail)

	_, reporterConfig := GinkgoConfiguration()
	reportsDir := os.Getenv("REPORTS_DUMP_DIR")
	if reportsDir == "" {
		reportsDir = os.TempDir()
	}
	reporterConfig.JUnitReport = filepath.Join(reportsDir, "smoke_junit.xml")

	RunSpecs(t, "Smoke", Label("smoke"), reporterConfig)
}
