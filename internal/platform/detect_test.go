package platform

import (
	"errors"
	"strings"
	"testing"

	"github.com/johnahull/amd-gpu-e2e/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func TestDetectServerGroupsError(t *testing.T) {
	dc := fakeDiscovery()
	dc.PrependReactor("get", "group", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	_, err := Detect(dc, "")
	if err == nil {
		t.Fatal("expected an error when ServerGroups() fails")
	}
	if !strings.Contains(err.Error(), "listing API groups") {
		t.Errorf("error %q should mention \"listing API groups\"", err.Error())
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error %q should wrap the underlying error", err.Error())
	}
}
