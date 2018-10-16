package provisioner

import (
	"fmt"
	"path/filepath"
)

func NewProvisioner(bundle []RemoteFileSpec, entrypoint string, s3Client S3ObjectPutter, s3URI string, pkgCacheDir string) *Provisioner {
	name := "default"

	prov := &Provisioner{
		Name:       name,
		Entrypoint: entrypoint,
		Bundle:     bundle,
		S3URI:      s3URI,
		CacheDir:   pkgCacheDir,
		S3:         s3Client,
		Loader:     &RemoteFileLoader{},
	}

	return prov
}

func (p *Provisioner) PrepareTransfer() (*Transfer, error) {
	name := p.Name

	pkgLocalPath := fmt.Sprintf("%s/%s.tgz", p.CacheDir, name)

	pkg := Package{
		RemoteFileSpec{
			Source: Source{Path: pkgLocalPath},
		},
		p.Bundle,
	}

	err := pkg.Create(p.Loader)
	if err != nil {
		return nil, fmt.Errorf("failed creating package: %v", err)
	}

	trans := Transfer{
		PackageFile: pkg.File,
		S3URI:       p.S3URI,
	}

	return &trans, nil
}

func (p *Provisioner) Send() error {
	trans, err := p.PrepareTransfer()
	if err != nil {
		return err
	}
	if err := trans.Send(p.S3); err != nil {
		return fmt.Errorf("failed sending package: %v", err)
	}
	return nil
}

func (p *Provisioner) RemoteCommand() (string, error) {
	trans, err := p.PrepareTransfer()
	if err != nil {
		return "", err
	}
	dstdir := "/var/run/coreos"
	return fmt.Sprintf(`run bash -c "%s" && tar zxvf %s -C / && %s`, trans.ReceiveCommand(dstdir), filepath.Join(dstdir, trans.PackageFile.Name()), p.Entrypoint), nil
}
