//go:build integration

package dra

import (
	"context"
	"errors"
	"os"
	"time"

	_ "github.com/johnahull/k8s-dra-harness/internal/adapters/defaults"
	"github.com/johnahull/k8s-dra-harness/internal/harness"
	"github.com/johnahull/k8s-dra-harness/internal/runconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("selected drivers", Ordered, func() {
	var runner *harness.Runner
	BeforeAll(func(ctx SpecContext) {
		path := os.Getenv("DRA_HARNESS_CONFIG")
		Expect(path).NotTo(BeEmpty(), "set DRA_HARNESS_CONFIG to a run YAML file")
		cfg, err := runconfig.Load(path)
		Expect(err).NotTo(HaveOccurred())
		runner, err = harness.New(cfg)
		Expect(err).NotTo(HaveOccurred())
		err = runner.Start(ctx)
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			err = errors.Join(err, runner.Cleanup(cleanupCtx))
			runner = nil
		}
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if runner == nil || !runner.Config.ShouldCleanup() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		Expect(runner.Cleanup(ctx)).To(Succeed())
	})

	It("publishes ready driver pods and DRA resources", func(ctx SpecContext) {
		for _, d := range runner.Drivers {
			By("checking " + d.Config.Name)
			Expect(runner.CheckDriver(ctx, d)).To(Succeed())
		}
	})

	It("runs checkout end-to-end suites when configured", func(ctx SpecContext) {
		for _, d := range runner.Drivers {
			if len(d.Config.UpstreamTests) == 0 {
				continue
			}
			By("running " + d.Config.Name + " upstream tests")
			Expect(runner.RunUpstreamTests(ctx, d)).To(Succeed())
		}
	})

	It("runs a live workload for each driver", func(ctx SpecContext) {
		for _, d := range runner.Drivers {
			By("allocating " + d.Config.Name)
			Expect(runner.RunWorkload(ctx, d)).To(Succeed())
		}
	})

	It("runs an AMD Operator device-plugin workload when selected alone", func(ctx SpecContext) {
		if runner.Config.Operator == nil || len(runner.Drivers) > 0 {
			Skip("requires an operator-only run")
		}
		Expect(runner.RunOperatorWorkload(ctx)).To(Succeed())
	})

	It("allocates a supported driver pair to one workload", func(ctx SpecContext) {
		if !runner.SupportsJointWorkload() {
			Skip("selected drivers have no joint workload")
		}
		Expect(runner.RunJointWorkload(ctx)).To(Succeed())
	})
})
