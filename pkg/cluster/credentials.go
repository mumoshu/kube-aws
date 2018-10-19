package cluster

import (
	"github.com/kubernetes-incubator/kube-aws/credential"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
)

func (s *Session) InitCredentials(cfg *Config, opts clusterapi.StackTemplateOptions) (*credential.CompactAssets, error) {
	if cfg.AssetsEncryptionEnabled() {
		kmsConfig := credential.NewKMSConfig(cfg.KMSKeyARN, s.ProvidedEncryptService, s.Session)
		compactAssets, err := credential.ReadOrCreateCompactAssets(opts.AssetsDir, cfg.ManageCertificates, cfg.Experimental.TLSBootstrap.Enabled, cfg.Experimental.KIAMSupport.Enabled, kmsConfig)
		if err != nil {
			return nil, err
		}

		return compactAssets, nil
	} else {
		rawAssets, err := credential.ReadOrCreateUnencryptedCompactAssets(opts.AssetsDir, cfg.ManageCertificates, cfg.Experimental.TLSBootstrap.Enabled, cfg.Experimental.KIAMSupport.Enabled)
		if err != nil {
			return nil, err
		}

		return rawAssets, nil
	}
}
