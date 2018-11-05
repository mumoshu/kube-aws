package cluster

import (
	"errors"
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/coreos/amiregistry"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi/derived"
	"strings"
)

func Compile(cfgRef *clusterapi.Cluster, opts clusterapi.ClusterOptions) (*Config, error) {
	c := &clusterapi.Cluster{}
	*c = *cfgRef

	// Import all the managed subnets from the network stack
	var err error
	c.Subnets, err = c.Subnets.ImportFromNetworkStackRetainingNames()
	if err != nil {
		return nil, fmt.Errorf("failed to import subnets from network stack: %v", err)
	}
	c.VPC = c.VPC.ImportFromNetworkStack()
	c.SetDefaults()

	config := Config{
		Cluster:          c,
		APIServerFlags:   clusterapi.CommandLineFlags{},
		APIServerVolumes: clusterapi.APIServerVolumes{},
		ControllerFlags:  clusterapi.CommandLineFlags{},
	}

	if c.AmiId == "" {
		var err error
		if config.AMI, err = amiregistry.GetAMI(config.Region.String(), config.ReleaseChannel); err != nil {
			return nil, fmt.Errorf("failed getting AMI for config: %v", err)
		}
	} else {
		config.AMI = c.AmiId
	}

	config.EtcdNodes, err = derived.NewEtcdNodes(c.Etcd.Nodes, config.EtcdCluster())
	if err != nil {
		return nil, fmt.Errorf("failed to derived etcd nodes configuration: %v", err)
	}

	// Populate top-level subnets to model
	if len(config.Subnets) > 0 {
		if config.Controller.MinControllerCount() > 0 && len(config.Controller.Subnets) == 0 {
			config.Controller.Subnets = config.Subnets
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

	if opts.S3URI != "" {
		c.S3URI = strings.TrimSuffix(opts.S3URI, "/")
	}

	s3Folders := clusterapi.NewS3Folders(c.S3URI, c.ClusterName)
	//conf.S3URI = s3Folders.ClusterExportedStacks().URI()
	c.KubeResourcesAutosave.S3Path = s3Folders.ClusterBackups().Path()

	if opts.SkipWait {
		enabled := false
		c.WaitSignal.EnabledOverride = &enabled
	}

	for i, np := range config.Worker.NodePools {
		if err := np.Taints.Validate(); err != nil {
			return nil, fmt.Errorf("invalid taints for node pool at index %d: %v", i, err)
		}
		if np.APIEndpointName == "" {
			if c.Worker.APIEndpointName == "" {
				if len(config.APIEndpoints) > 1 {
					return nil, errors.New("worker.apiEndpointName can be omitted only when there's only 1 api endpoint under apiEndpoints")
				}
				np.APIEndpointName = config.APIEndpoints.GetDefault().Name
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

		config.Worker.NodePools[i] = np
	}

	return &config, nil
}
