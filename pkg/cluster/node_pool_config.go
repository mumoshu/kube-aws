package cluster

import (
	"fmt"
	"strings"

	"errors"

	"github.com/kubernetes-incubator/kube-aws/cfnresource"
	"github.com/kubernetes-incubator/kube-aws/credential"
	"github.com/kubernetes-incubator/kube-aws/logger"
	"github.com/kubernetes-incubator/kube-aws/naming"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi/derived"
)

type Ref struct {
	PoolName string
}

type NodePoolConfig struct {
	clusterapi.WorkerNodePool `yaml:",inline"`

	MainClusterSettings
	// APIEndpoint is the k8s api endpoint to which worker nodes in this node pool communicate
	APIEndpoint            derived.APIEndpoint
	ProvidedEncryptService credential.KMSEncryptionService
	clusterapi.UnknownKeys `yaml:",inline"`
}

type MainClusterSettings struct {
	EtcdNodes             []derived.EtcdNode
	KubeResourcesAutosave clusterapi.KubeResourcesAutosave
}

// NestedStackName returns a sanitized name of this node pool which is usable as a valid cloudformation nested stack name
func (c NodePoolConfig) NestedStackName() string {
	// Convert stack name into something valid as a cfn resource name or
	// we'll end up with cfn errors like "Template format error: Resource name test5-controlplane is non alphanumeric"
	return naming.FromStackToCfnResource(c.StackName())
}

func (c *NodePoolConfig) ExternalDNSName() string {
	logger.Warn("WARN: ExternalDNSName is deprecated and will be removed in v0.9.7. Please use APIEndpoint.Name instead")
	return c.APIEndpoint.DNSName
}

// APIEndpointURL is the url of the API endpoint which is written in cloud-config-worker and used by kubelets in worker nodes
// to access the apiserver
func (c NodePoolConfig) APIEndpointURL() string {
	return fmt.Sprintf("https://%s", c.APIEndpoint.DNSName)
}

func (c NodePoolConfig) APIEndpointURLPort() string {
	return c.APIEndpointURL() + ":443"
}

func (c NodePoolConfig) AWSIAMAuthenticatorClusterIDRef() string {
	var rawClusterId string
	if c.Kubernetes.Authentication.AWSIAM.ClusterID != "" {
		rawClusterId = c.Kubernetes.Authentication.AWSIAM.ClusterID
	} else {
		rawClusterId = c.ClusterName
	}
	return fmt.Sprintf(`"%s"`, rawClusterId)
}

func (c NodePoolConfig) NodeLabels() clusterapi.NodeLabels {
	labels := c.NodeSettings.NodeLabels
	if c.ClusterAutoscalerSupport.Enabled {
		labels["kube-aws.coreos.com/cluster-autoscaler-supported"] = "true"
	}
	return labels
}

func (c NodePoolConfig) FeatureGates() clusterapi.FeatureGates {
	gates := c.NodeSettings.FeatureGates
	if c.Gpu.Nvidia.IsEnabledOn(c.InstanceType) {
		gates["Accelerators"] = "true"
	}
	if c.Experimental.GpuSupport.Enabled {
		gates["DevicePlugins"] = "true"
	}
	if c.Kubelet.RotateCerts.Enabled {
		gates["RotateKubeletClientCertificate"] = "true"
	}
	return gates
}

func (c NodePoolConfig) WorkerDeploymentSettings() NodePoolDeploymentSettings {
	return NodePoolDeploymentSettings{
		WorkerNodePool:     c.WorkerNodePool,
		Experimental:       c.Experimental,
		DeploymentSettings: c.DeploymentSettings,
	}
}

func (c NodePoolConfig) ValidateInputs() error {
	if err := c.DeploymentSettings.ValidateInputs(c.NodePoolName); err != nil {
		return err
	}

	if err := c.WorkerNodePool.ValidateInputs(); err != nil {
		return err
	}

	if len(c.Subnets) > 1 && c.Autoscaling.ClusterAutoscaler.Enabled {
		return errors.New("cluster-autoscaler can't be enabled for a node pool with 2 or more subnets because allowing so" +
			"results in unreliability while scaling nodes out. ")
	}

	return nil
}

func (c NodePoolConfig) validate() error {
	if _, err := c.KubeClusterSettings.Validate(); err != nil {
		return err
	}

	if err := c.WorkerNodePool.Validate(c.Experimental); err != nil {
		return err
	}

	if err := c.DeploymentSettings.ValidateNodePool(c.NodePoolName); err != nil {
		return err
	}

	if err := c.WorkerDeploymentSettings().Validate(); err != nil {
		return err
	}

	if err := c.Experimental.Validate(c.NodePoolName); err != nil {
		return err
	}

	if err := c.NodeSettings.Validate(); err != nil {
		return err
	}

	clusterNamePlaceholder := "<my-cluster-name>"
	nestedStackNamePlaceHolder := "<my-nested-stack-name>"
	replacer := strings.NewReplacer(clusterNamePlaceholder, "", nestedStackNamePlaceHolder, "")
	simulatedLcName := fmt.Sprintf("%s-%s-1N2C4K3LLBEDZ-%sLC-BC2S9P3JG2QD", clusterNamePlaceholder, nestedStackNamePlaceHolder, c.LogicalName())
	limit := 63 - len(replacer.Replace(simulatedLcName))
	if c.Experimental.AwsNodeLabels.Enabled && len(c.ClusterName+c.NodePoolName) > limit {
		return fmt.Errorf("awsNodeLabels can't be enabled for node pool because the total number of characters in clusterName(=\"%s\") + node pool's name(=\"%s\") exceeds the limit of %d", c.ClusterName, c.NodePoolName, limit)
	}

	if len(c.WorkerNodePool.IAMConfig.Role.Name) > 0 {
		if e := cfnresource.ValidateStableRoleNameLength(c.ClusterName, c.WorkerNodePool.IAMConfig.Role.Name, c.Region.String()); e != nil {
			return e
		}
	} else {
		if e := cfnresource.ValidateUnstableRoleNameLength(c.ClusterName, c.NestedStackName(), c.WorkerNodePool.IAMConfig.Role.Name, c.Region.String()); e != nil {
			return e
		}
	}

	return nil
}

// StackName returns the logical name of a CloudFormation stack resource in a root stack template
// This is not needed to be unique in an AWS account because the actual name of a nested stack is generated randomly
// by CloudFormation by including the logical name.
// This is NOT intended to be used to reference stack name from cloud-config as the target of awscli or cfn-bootstrap-tools commands e.g. `cfn-init` and `cfn-signal`
func (c NodePoolConfig) StackName() string {
	return c.NodePoolName
}

func (c NodePoolConfig) StackNameEnvFileName() string {
	return "/etc/environment"
}

func (c NodePoolConfig) StackNameEnvVarName() string {
	return "KUBE_AWS_STACK_NAME"
}

func (c NodePoolConfig) VPCRef() (string, error) {
	igw := c.InternetGateway
	// When HasIdentifier returns true, it means the VPC already exists, and we can reference it directly by ID
	if !c.VPC.HasIdentifier() {
		// Otherwise import the VPC ID from the control-plane stack
		igw.IDFromStackOutput = `{"Fn::Sub" : "${NetworkStackName}-VPC"}`
	}
	return igw.RefOrError(func() (string, error) {
		return "", fmt.Errorf("[BUG] Tried to reference VPC by its logical name")
	})
}

func (c NodePoolConfig) SecurityGroupRefs() []string {
	refs := c.WorkerDeploymentSettings().WorkerSecurityGroupRefs()

	refs = append(
		refs,
		// The security group assigned to worker nodes to allow communication to etcd nodes and controller nodes
		// which is created and maintained in the main cluster and then imported to node pools.
		`{"Fn::ImportValue" : {"Fn::Sub" : "${NetworkStackName}-WorkerSecurityGroup"}}`,
	)

	return refs
}
