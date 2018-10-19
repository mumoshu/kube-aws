package clusterapi

type PluginConfigs map[string]PluginConfig

type PluginConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
	Values  `yaml:",inline"`
}
