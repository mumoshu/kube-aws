package cluster

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-incubator/kube-aws/cfnstack"
	"github.com/kubernetes-incubator/kube-aws/credential"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/provisioner"
)

type tmplCtx = interface{}

type Stack struct {
	archivedFiles   []provisioner.RemoteFileSpec
	NodeProvisioner *provisioner.Provisioner

	StackName   string
	S3URI       string
	ClusterName string
	Region      clusterapi.Region

	Config         *Config
	NodePoolConfig *NodePoolConfig

	tmplCtx

	clusterapi.StackTemplateOptions
	UserData          map[string]clusterapi.UserData
	CfnInitConfigSets map[string]interface{}
	ExtraCfnResources map[string]interface{}
	AssetsConfig      *credential.CompactAssets
	assets            cfnstack.Assets
}

type ec2Service interface {
	CreateVolume(*ec2.CreateVolumeInput) (*ec2.Volume, error)
	DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	DescribeKeyPairs(*ec2.DescribeKeyPairsInput) (*ec2.DescribeKeyPairsOutput, error)
}

type Session struct {
	Session *session.Session

	ProvidedEncryptService  credential.KMSEncryptionService
	ProvidedCFInterrogator  cfnstack.CFInterrogator
	ProvidedEC2Interrogator cfnstack.EC2Interrogator
}

// An EtcdTmplCtx contains configuration settings/options mixed with existing state in a way that can be
// consumed by stack and cloud-config templates.
type EtcdTmplCtx struct {
	*Stack
	*Config
	clusterapi.EtcdExistingState
}

// DefaultTmplCtx is used for rendering controller and network stacks and userdata
type DefaultTmplCtx struct {
	*Stack
	*Config
}

// WorkerTmplCtx is used for rendering worker stacks and userdata
type WorkerTmplCtx struct {
	*Stack
	*NodePoolConfig
}
