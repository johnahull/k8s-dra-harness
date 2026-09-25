package platform

import (
	"testing"

	"github.com/johnahull/amd-ci/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
)

func fakeDiscovery(groupVersions ...string) *fakediscovery.FakeDiscovery {
	var resources []*metav1.APIResourceList
	for _, gv := range groupVersions {
		resources = append(resources, &metav1.APIResourceList{GroupVersion: gv})
	}

	return &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{Resources: resources}}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		groups   []string
		override config.Platform
		want     config.Platform
		wantErr  bool
	}{
		{name: "openshift by API group", groups: []string{"v1", "apps/v1", "config.openshift.io/v1"}, want: config.PlatformOpenShift},
		{name: "kubernetes by absence", groups: []string{"v1", "apps/v1"}, want: config.PlatformKubernetes},
		{name: "override wins", groups: []string{"config.openshift.io/v1"}, override: config.PlatformKubernetes, want: config.PlatformKubernetes},
		{name: "invalid override", override: "rancher", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(fakeDiscovery(tt.groups...), tt.override)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
