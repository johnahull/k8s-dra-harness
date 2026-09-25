//go:build integration

package smoke

import (
	"github.com/johnahull/k8s-dra-harness/pkg/clients"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Cluster smoke", Label("smoke"), func() {
	It("connects and lists nodes", func(ctx SpecContext) {
		client, err := clients.New("")
		Expect(err).NotTo(HaveOccurred())
		nodes, err := client.K8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes.Items).NotTo(BeEmpty())
		GinkgoWriter.Printf("nodes=%d\n", len(nodes.Items))
	})

	It("serves the GA DRA API", func() {
		client, err := clients.New("")
		Expect(err).NotTo(HaveOccurred())
		_, err = client.Discovery.ServerResourcesForGroupVersion("resource.k8s.io/v1")
		Expect(err).NotTo(HaveOccurred())
	})
})
