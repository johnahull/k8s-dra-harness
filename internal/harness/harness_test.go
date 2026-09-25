package harness

import "testing"

func TestValueOverrideKeepsUserConfig(t *testing.T) {
	user := map[string]any{"deviceConfig": map[string]any{"spec": map[string]any{"devicePlugin": map[string]any{"enableDevicePlugin": true}}}, "image": map[string]any{"pullPolicy": "Always"}}
	values := cloneValues(user)
	setValue(values, false, "deviceConfig", "spec", "devicePlugin", "enableDevicePlugin")
	values["image"] = mergeImage(values["image"], "quay.io/team/driver", "test")
	original := user["deviceConfig"].(map[string]any)["spec"].(map[string]any)["devicePlugin"].(map[string]any)["enableDevicePlugin"]
	if original != true {
		t.Fatal("operator override changed the caller's config")
	}
	image := values["image"].(map[string]any)
	if image["repository"] != "quay.io/team/driver" || image["tag"] != "test" || image["pullPolicy"] != "Always" {
		t.Fatalf("image override lost user values: %+v", image)
	}
}
