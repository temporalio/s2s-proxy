package config

type (
	MockConfigProvider struct {
		config S2SProxyConfig
	}
)

var (
	EmptyConfigProvider MockConfigProvider
)

func NewMockConfigProvider(config S2SProxyConfig) *MockConfigProvider {
	return &MockConfigProvider{config: config}
}

func (mc *MockConfigProvider) GetS2SProxyConfig() S2SProxyConfig {
	return mc.config
}
