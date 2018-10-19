package clusterapi

type PKI struct {
	KeyPairs []KeyPairSpec `yaml:"keypairs,omitempty"`
}
