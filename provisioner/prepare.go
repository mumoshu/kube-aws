package provisioner

import (
	"fmt"
)

func NewTarballingProvisioner(bundle []RemoteFileSpec, entrypoint string, s3DirURI string, pkgCacheDir string) *Provisioner {
	name := "bundle.tgz"

	prov := &Provisioner{
		Name:          name,
		Entrypoint:    entrypoint,
		Bundle:        bundle,
		S3DirURI:      s3DirURI,
		LocalCacheDir: pkgCacheDir,
	}

	return prov
}

func (p *Provisioner) GetTransferredFile() TransferredFile {
	pkgFileName := p.Name

	pkgLocalPath := fmt.Sprintf("%s/%s", p.LocalCacheDir, pkgFileName)

	archive := RemoteFileSpec{
		Path:   fmt.Sprintf("/var/run/coreos/%s", pkgFileName),
		Source: Source{Path: pkgLocalPath},
	}

	return TransferredFile{
		archive,
		p.S3DirURI,
	}
}

func (p *Provisioner) CreateTransferredFile(loader *RemoteFileLoader) (*TransferredFile, error) {
	transferredFile := p.GetTransferredFile()

	err := CreateTarGzArchive(transferredFile.RemoteFileSpec, p.Bundle, loader)
	if err != nil {
		return nil, fmt.Errorf("failed creating package: %v", err)
	}

	return &transferredFile, nil
}

func (p *Provisioner) Send(s3Client S3ObjectPutter) error {
	trans := p.GetTransferredFile()
	if err := trans.Send(s3Client); err != nil {
		return fmt.Errorf("failed sending package: %v", err)
	}
	return nil
}

func (p *Provisioner) RemoteCommand() (string, error) {
	trans := p.GetTransferredFile()

	return fmt.Sprintf(`run bash -c "%s" && tar zxvf %s -C / && %s`, trans.ReceiveCommand(), trans.Path, p.Entrypoint), nil
}
