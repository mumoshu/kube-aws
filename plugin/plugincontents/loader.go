package plugincontents

import (
	"path/filepath"

	"fmt"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/provisioner"
)

type PluginFileLoader struct {
	p *clusterapi.Plugin

	FileLoader *provisioner.RemoteFileLoader
}

func NewPluginFileLoader(p *clusterapi.Plugin) *PluginFileLoader {
	return &PluginFileLoader{
		p: p,
	}
}

func (l *PluginFileLoader) String(f provisioner.RemoteFileSpec) (string, error) {
	if f.Source.Path != "" {
		f.Source.Path = filepath.Join("plugins", l.p.Name, f.Source.Path)
	}

	loaded, err := l.FileLoader.Load(f)
	if err != nil {
		return "", err
	}

	res := loaded.Content.String()

	if f.Source.Path != "" && len(res) == 0 {
		return "", fmt.Errorf("[bug] empty file loaded from %s", f.Source.Path)
	}

	return res, nil
}
