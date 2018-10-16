package pki

import (
	"fmt"
	"path/filepath"
)

func (spec KeyPairSpec) EncryptedKeyPath() string {
	return fmt.Sprintf("%s.enc", spec.KeyPath())
}

func (spec KeyPairSpec) KeyPath() string {
	return filepath.Join("credentials", fmt.Sprintf("%s-key.pem", spec.Name))
}

func (spec KeyPairSpec) CertPath() string {
	return filepath.Join("credentials", fmt.Sprintf("%s.pem", spec.Name))
}
