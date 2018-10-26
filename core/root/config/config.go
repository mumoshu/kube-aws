package config

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/go-yaml/yaml"
	"github.com/kubernetes-incubator/kube-aws/pkg/cluster"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/plugin"
	"github.com/kubernetes-incubator/kube-aws/plugin/clusterextension"
)

type InitialConfig struct {
	AmiId            string
	AvailabilityZone string
	ClusterName      string
	ExternalDNSName  string
	HostedZoneID     string
	KMSKeyARN        string
	KeyName          string
	NoRecordSet      bool
	Region           clusterapi.Region
	S3URI            string
}

type UnmarshalledConfig struct {
	clusterapi.Cluster     `yaml:",inline"`
	clusterapi.UnknownKeys `yaml:",inline"`
}

type Config struct {
	*cluster.Config
	NodePools              []*cluster.NodePoolConfig
	Plugins                []*clusterapi.Plugin
	clusterapi.UnknownKeys `yaml:",inline"`

	Extras *clusterextension.ClusterExtension
}

type unknownKeysSupport interface {
	FailWhenUnknownKeysFound(keyPath string) error
}

type unknownKeyValidation struct {
	unknownKeysSupport
	keyPath string
}

func newDefaultUnmarshalledConfig() *UnmarshalledConfig {
	return &UnmarshalledConfig{
		Cluster: *clusterapi.NewDefaultCluster(),
	}
}

func unmarshalConfig(data []byte) (*UnmarshalledConfig, error) {
	c := newDefaultUnmarshalledConfig()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	c.HyperkubeImage.Tag = c.K8sVer

	return c, nil
}

func ConfigFromBytes(data []byte, plugins []*clusterapi.Plugin) (*Config, error) {
	c, err := unmarshalConfig(data)
	if err != nil {
		return nil, err
	}

	cpCluster := &c.Cluster
	if err := cpCluster.Load(cluster.ControlPlaneStackName); err != nil {
		return nil, err
	}

	extras := clusterextension.NewExtrasFromPlugins(plugins, c.PluginConfigs)

	opts := clusterapi.ClusterOptions{
		S3URI: c.S3URI,
		// TODO
		SkipWait: false,
	}

	cpConfig, err := cluster.Compile(cpCluster, opts)
	if err != nil {
		return nil, err
	}

	nodePools := c.NodePools

	anyNodePoolIsMissingAPIEndpointName := true
	for _, np := range nodePools {
		if np.APIEndpointName == "" {
			anyNodePoolIsMissingAPIEndpointName = true
			break
		}
	}

	if len(cpConfig.APIEndpoints) > 1 && c.Worker.APIEndpointName == "" && anyNodePoolIsMissingAPIEndpointName {
		return nil, errors.New("worker.apiEndpointName must not be empty when there're 2 or more API endpoints under the key `apiEndpoints` and one of worker.nodePools[] are missing apiEndpointName")
	}

	if c.Worker.APIEndpointName != "" {
		if _, err := cpConfig.APIEndpoints.FindByName(c.APIEndpointName); err != nil {
			return nil, fmt.Errorf("invalid value for worker.apiEndpointName: no API endpoint named \"%s\" found", c.APIEndpointName)
		}
	}

	nps := []*cluster.NodePoolConfig{}
	for i, np := range nodePools {
		if err := np.Taints.Validate(); err != nil {
			return nil, fmt.Errorf("invalid taints for node pool at index %d: %v", i, err)
		}
		if np.APIEndpointName == "" {
			if c.Worker.APIEndpointName == "" {
				if len(cpConfig.APIEndpoints) > 1 {
					return nil, errors.New("worker.apiEndpointName can be omitted only when there's only 1 api endpoint under apiEndpoints")
				}
				np.APIEndpointName = cpConfig.APIEndpoints.GetDefault().Name
			} else {
				np.APIEndpointName = c.Worker.APIEndpointName
			}
		}

		if np.NodePoolRollingStrategy != "Parallel" && np.NodePoolRollingStrategy != "Sequential" {
			if c.Worker.NodePoolRollingStrategy != "" && (c.Worker.NodePoolRollingStrategy == "Sequential" || c.Worker.NodePoolRollingStrategy == "Parallel") {
				np.NodePoolRollingStrategy = c.Worker.NodePoolRollingStrategy
			} else {
				np.NodePoolRollingStrategy = "Parallel"
			}
		}

		npConf, err := cluster.NodePoolCompile(np, cpConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid node pool at index %d: %v", i, err)
		}

		if np.Autoscaling.ClusterAutoscaler.Enabled && !cpConfig.Addons.ClusterAutoscaler.Enabled {
			return nil, errors.New("Autoscaling with cluster-autoscaler can't be enabled for node pools because " +
				"you didn't enabled the cluster-autoscaler addon. Enable it by turning on `addons.clusterAutoscaler.enabled`")
		}

		if err := failFastWhenUnknownKeysFound([]unknownKeyValidation{
			{np, fmt.Sprintf("worker.nodePools[%d]", i)},
			{np.AutoScalingGroup, fmt.Sprintf("worker.nodePools[%d].autoScalingGroup", i)},
			{np.Autoscaling.ClusterAutoscaler, fmt.Sprintf("worker.nodePools[%d].autoscaling.clusterAutoscaler", i)},
			{np.SpotFleet, fmt.Sprintf("worker.nodePools[%d].spotFleet", i)},
		}); err != nil {
			return nil, err
		}

		nps = append(nps, npConf)
	}

	cfg := &Config{Config: cpConfig, NodePools: nps}

	validations := []unknownKeyValidation{
		{c, ""},
		{c.Worker, "worker"},
		{c.Etcd, "etcd"},
		{c.Etcd.RootVolume, "etcd.rootVolume"},
		{c.Etcd.DataVolume, "etcd.dataVolume"},
		{c.Controller, "controller"},
		{c.Controller.AutoScalingGroup, "controller.autoScalingGroup"},
		{c.Controller.Autoscaling.ClusterAutoscaler, "controller.autoscaling.clusterAutoscaler"},
		{c.Controller.RootVolume, "controller.rootVolume"},
		{c.Experimental, "experimental"},
		{c.Addons, "addons"},
		{c.Addons.Rescheduler, "addons.rescheduler"},
		{c.Addons.ClusterAutoscaler, "addons.clusterAutoscaler"},
		{c.Addons.MetricsServer, "addons.metricsServer"},
	}

	for i, np := range c.Worker.NodePools {
		validations = append(validations, unknownKeyValidation{np, fmt.Sprintf("worker.nodePools[%d]", i)})
		validations = append(validations, unknownKeyValidation{np.RootVolume, fmt.Sprintf("worker.nodePools[%d].rootVolume", i)})

	}

	for i, endpoint := range c.APIEndpointConfigs {
		validations = append(validations, unknownKeyValidation{endpoint, fmt.Sprintf("apiEndpoints[%d]", i)})
	}

	if err := failFastWhenUnknownKeysFound(validations); err != nil {
		return nil, err
	}

	cfg.Plugins = plugins
	cfg.Extras = &extras

	return cfg, nil
}

func failFastWhenUnknownKeysFound(vs []unknownKeyValidation) error {
	for _, v := range vs {
		if err := v.unknownKeysSupport.FailWhenUnknownKeysFound(v.keyPath); err != nil {
			return err
		}
	}
	return nil
}

//func ConfigFromBytesWithStubs(data []byte, plugins []*clusterapi.Plugin, encryptService credential.KMSEncryptionService, cf cfnstack.CFInterrogator, ec cfnstack.EC2Interrogator) (*Config, error) {
//	c, err := ConfigFromBytes(data, plugins)
//	if err != nil {
//		return nil, err
//	}
//	c.ProvidedEncryptService = encryptService
//	c.ProvidedCFInterrogator = cf
//	c.ProvidedEC2Interrogator = ec
//
//	// Uses the same encrypt service for node pools for consistency
//	for _, p := range c.NodePools {
//		p.ProvidedEncryptService = encryptService
//	}
//
//	return c, nil
//}

func ConfigFromFile(configPath string) (*Config, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	plugins, err := plugin.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load plugins: %v", err)
	}

	c, err := ConfigFromBytes(data, plugins)
	if err != nil {
		return nil, fmt.Errorf("failed loading %s: %v", configPath, err)
	}

	return c, nil
}

func (c *Config) RootStackName() string {
	return c.ClusterName
}
