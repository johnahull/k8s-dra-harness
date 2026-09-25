package config

import (
	"fmt"
	"strings"
)

// KeyValues is a map read from an env var formatted as "k=v,k=v".
// Only the first '=' in a pair separates key from value, so values may contain '='.
type KeyValues map[string]string

// Decode implements envconfig.Decoder.
func (kv *KeyValues) Decode(value string) error {
	m := KeyValues{}

	if strings.TrimSpace(value) == "" {
		*kv = m

		return nil
	}

	for _, pair := range strings.Split(value, ",") {
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)

		if !ok || k == "" {
			return fmt.Errorf("invalid key=value pair %q", pair)
		}

		m[k] = strings.TrimSpace(v)
	}

	*kv = m

	return nil
}
