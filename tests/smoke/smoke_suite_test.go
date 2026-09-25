//go:build integration

package smoke

import (
	"path/filepath"
	"testing"

	. "github.com/johnahull/amd-ci/internal/inittools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(Fail)

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = filepath.Join(Config.ReportsDir, "smoke_junit.xml")

	RunSpecs(t, "Smoke", Label("smoke"), reporterConfig)
}
