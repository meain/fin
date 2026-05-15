package config

import "strings"

// ResolveModel walks model alias chains (capped at 10 hops) and splits the
// resolved name into provider and model parts. Returns ("", name) when the
// name has no slash.
func ResolveModel(model string, c *Config) (providerName, modelName string) {
	for i := 0; i < 10; i++ {
		if resolved, ok := c.ModelAliases[model]; ok {
			model = resolved
		} else {
			break
		}
	}
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", model
}
