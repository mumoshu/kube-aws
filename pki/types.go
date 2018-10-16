package pki

import (
	"crypto/rsa"
	"crypto/x509"
	"time"
)

type KeyPairSpec struct {
	Name         string        `yaml:"name"`
	CommonName   string        `yaml:"commonName"`
	Organization string        `yaml:"organization"`
	Duration     time.Duration `yaml:"duration"`
	DNSNames     []string      `yaml:dnsNames"`
	IPAddresses  []string      `yaml:"ipAddresses"`
	Usages       []string      `yaml:"usages"`
	// Signer is the name of the keypair for the private key used to sign the cert
	Signer string `yaml:"signer"`
}

// KeyPair is the TLS public certificate PEM file and its associated private key PEM file that is
// used by kube-aws and its plugins
type KeyPair struct {
	Key  *rsa.PrivateKey
	Cert *x509.Certificate

	id string

	keyPem  []byte
	certPem []byte
}
