package clusterapi

type EtcdNode struct {
	Name string `yaml:"name,omitempty"`
	FQDN string `yaml:"fqdn,omitempty"`
}
