package agent

import (
	"context"
	"fmt"

	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/provider"
	t "github.com/meain/fin/internal/types"
)

// modelInjector wraps a provider.Provider and overrides the Model field of
// every completion request. Providers accept a Model in the request body;
// the model name comes from the resolved alias and isn't known until after
// the provider is constructed.
type modelInjector struct {
	provider provider.Provider
	model    string
}

func (m *modelInjector) StreamCompletion(ctx context.Context, req t.CompletionRequest) (provider.Stream, error) {
	req.Model = m.model
	return m.provider.StreamCompletion(ctx, req)
}

// NewProviderInjector wraps a Provider with a fixed model name. Exposed so
// the CLI can construct a top-level provider; the same helper is used
// internally by GenerateTitle and runSubagent.
func NewProviderInjector(p provider.Provider, model string) provider.Provider {
	return &modelInjector{provider: p, model: model}
}

// newProviderForModel resolves the model alias, instantiates the provider,
// and wraps it in a modelInjector. Returns the wrapped provider plus the
// fully-resolved "provider/model" string for tagging messages.
func newProviderForModel(cfg *config.Config, modelStr string) (provider.Provider, string, error) {
	providerName, modelName := config.ResolveModel(modelStr, cfg)
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, "", fmt.Errorf("unknown provider %q", providerName)
	}
	raw, err := provider.New(providerName, provider.Config{
		BaseURL:   providerCfg.BaseURL,
		APIKeyEnv: providerCfg.APIKeyEnv,
		Headers:   providerCfg.Headers,
	})
	if err != nil {
		return nil, "", err
	}
	return &modelInjector{provider: raw, model: modelName}, providerName + "/" + modelName, nil
}
