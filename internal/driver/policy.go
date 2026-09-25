package driver

// ImageValues merges a tagged image into chart values without changing the caller's map.
func ImageValues(values map[string]any, image string) (map[string]any, error) {
	repo, tag, err := SplitImage(image)
	if err != nil {
		return nil, err
	}
	out := cloneValues(values)
	imageMap, _ := out["image"].(map[string]any)
	if imageMap == nil {
		imageMap = map[string]any{}
	}
	imageMap["repository"], imageMap["tag"] = repo, tag
	out["image"] = imageMap
	return out, nil
}

func cloneValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]any); ok {
			out[k] = cloneValues(nested)
		} else {
			out[k] = v
		}
	}
	return out
}
