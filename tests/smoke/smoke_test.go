//go:build integration

package smoke

import (
	"github.com/johnahull/amd-gpu-e2e/internal/config"
	"github.com/johnahull/amd-gpu-e2e/internal/discovery"
	. "github.com/johnahull/amd-gpu-e2e/internal/inittools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
)

var _ = Describe("Cluster smoke", Label("smoke"), func() {
	It("detects the platform and applies defaults", func() {
		Expect(Config.Platform).To(BeElementOf(config.PlatformOpenShift, config.PlatformKubernetes))
		Expect(Config.Namespace).NotTo(BeEmpty())
		GinkgoWriter.Printf("platform=%s namespace=%s driverMode=%s draSource=%s\n",
			Config.Platform, Config.Namespace, Config.Driver.Mode, Config.DRA.Source)
	})

	It("finds AMD GPU nodes", func(ctx SpecContext) {
		nodes, err := discovery.GPUNodes(ctx, APIClient)
		Expect(err).NotTo(HaveOccurred())

		if len(nodes) == 0 {
			Skip("no AMD GPU nodes: no amd-gpu label and no NFD PCI vendor 1002 label")
		}

		for _, n := range nodes {
			GinkgoWriter.Printf("AMD GPU node: %s\n", n.Name)
		}
	})

	It("reads DRA devices when resource.k8s.io/v1 is served", func(ctx SpecContext) {
		counts, err := discovery.DevicesByNode(ctx, APIClient)
		if meta.IsNoMatchError(err) {
			Skip("resource.k8s.io/v1 is not served by this cluster")
		}

		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("AMD DRA devices by node: %v\n", counts)
	})
})
