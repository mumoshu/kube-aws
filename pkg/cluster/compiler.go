package cluster

import (
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/coreos/amiregistry"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi/derived"
	"github.com/kubernetes-incubator/kube-aws/plugin/clusterextension"
	"strings"
)

func Preprocess(c *clusterapi.Cluster) (*Config, error) {
	config := Config{
		Cluster:          c,
		APIServerFlags:   clusterapi.APIServerFlags{},
		APIServerVolumes: clusterapi.APIServerVolumes{},
	}

	if c.AmiId == "" {
		var err error
		if config.AMI, err = amiregistry.GetAMI(config.Region.String(), config.ReleaseChannel); err != nil {
			return nil, fmt.Errorf("failed getting AMI for config: %v", err)
		}
	} else {
		config.AMI = c.AmiId
	}

	var err error
	config.EtcdNodes, err = derived.NewEtcdNodes(c.Etcd.Nodes, config.EtcdCluster())
	if err != nil {
		return nil, fmt.Errorf("failed to derived etcd nodes configuration: %v", err)
	}

	// Populate top-level subnets to model
	if len(config.Subnets) > 0 {
		if config.ControllerSettings.MinControllerCount() > 0 && len(config.ControllerSettings.Subnets) == 0 {
			config.ControllerSettings.Subnets = config.Subnets
		}
	}

	apiEndpoints, err := derived.NewAPIEndpoints(c.APIEndpointConfigs, c.Subnets)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster: %v", err)
	}

	config.APIEndpoints = apiEndpoints

	apiEndpointNames := []string{}
	for _, e := range apiEndpoints {
		apiEndpointNames = append(apiEndpointNames, e.Name)
	}

	var adminAPIEndpoint derived.APIEndpoint
	if c.AdminAPIEndpointName != "" {
		found, err := apiEndpoints.FindByName(c.AdminAPIEndpointName)
		if err != nil {
			return nil, fmt.Errorf("failed to find an API endpoint named \"%s\": %v", c.AdminAPIEndpointName, err)
		}
		adminAPIEndpoint = *found
	} else {
		if len(apiEndpoints) > 1 {
			return nil, fmt.Errorf(
				"adminAPIEndpointName must not be empty when there's 2 or more api endpoints under the key `apiEndpoints`. Specify one of: %s",
				strings.Join(apiEndpointNames, ", "),
			)
		}
		adminAPIEndpoint = apiEndpoints.GetDefault()
	}
	config.AdminAPIEndpoint = adminAPIEndpoint

	return &config, nil
}

func Compile(cfgRef *clusterapi.Cluster, opts clusterapi.ClusterOptions, extras clusterextension.ClusterExtension) (*Config, error) {
	cfg := &clusterapi.Cluster{}
	*cfg = *cfgRef

	// Import all the managed subnets from the network stack
	var err error
	cfg.Subnets, err = cfg.Subnets.ImportFromNetworkStackRetainingNames()
	if err != nil {
		return nil, fmt.Errorf("failed to import subnets from network stack: %v", err)
	}
	cfg.VPC = cfg.VPC.ImportFromNetworkStack()
	cfg.SetDefaults()

	conf, err := Preprocess(cfgRef)
	if err != nil {
		return nil, err
	}

	if opts.S3URI != "" {
		conf.S3URI = strings.TrimSuffix(opts.S3URI, "/")
	}

	s3Folders := clusterapi.NewS3Folders(conf.S3URI, conf.ClusterName)
	//conf.S3URI = s3Folders.ClusterExportedStacks().URI()
	conf.KubeResourcesAutosave.S3Path = s3Folders.ClusterBackups().Path()

	if opts.SkipWait {
		enabled := false
		conf.WaitSignal.EnabledOverride = &enabled
	}

	extraController, err := extras.Controller(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load controller node extras from plugins: %v", err)
	}

	if len(conf.Kubelet.Kubeconfig) == 0 {
		conf.Kubelet.Kubeconfig = extraController.Kubeconfig
	}
	conf.Kubelet.Mounts = append(conf.Kubelet.Mounts, extraController.KubeletVolumeMounts...)
	conf.APIServerFlags = append(conf.APIServerFlags, extraController.APIServerFlags...)
	conf.APIServerVolumes = append(conf.APIServerVolumes, extraController.APIServerVolumes...)
	conf.Controller.CustomSystemdUnits = append(conf.Controller.CustomSystemdUnits, extraController.SystemdUnits...)
	conf.Controller.CustomFiles = append(conf.Controller.CustomFiles, extraController.Files...)
	conf.Controller.IAMConfig.Policy.Statements = append(conf.Controller.IAMConfig.Policy.Statements, extraController.IAMPolicyStatements...)
	conf.KubeAWSVersion = VERSION
	for k, v := range extraController.NodeLabels {
		conf.Controller.NodeLabels[k] = v
	}
	conf.HelmReleaseFilesets = extraController.HelmReleaseFilesets
	conf.KubernetesManifestFiles = extraController.KubernetesManifestFiles

	if len(conf.StackTags) == 0 {
		conf.StackTags = make(map[string]string, 1)
	}
	conf.StackTags["kube-aws:version"] = VERSION

	return conf, nil
}
