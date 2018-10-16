package pluginmodel

import "github.com/kubernetes-incubator/kube-aws/pki"

type PKI struct {
	KeyPairs []pki.KeyPairSpec `yaml:"keypairs,omitempty"`
}
