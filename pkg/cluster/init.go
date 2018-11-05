package cluster

import (
	"fmt"
	"github.com/go-yaml/yaml"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"io/ioutil"
)

func ClusterFromFile(filename string) (*clusterapi.Cluster, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	c, err := ClusterFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("file %s: %v", filename, err)
	}

	return c, nil
}

// ClusterFromBytes Necessary for unit tests, which store configs as hardcoded strings
func ClusterFromBytes(data []byte) (*clusterapi.Cluster, error) {
	c := clusterapi.NewDefaultCluster()

	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("failed to parse cluster: %v", err)
	}

	c.HyperkubeImage.Tag = c.K8sVer

	if err := c.Load(ControlPlaneStackName); err != nil {
		return c, err
	}

	return c, nil
}
