package config

import (
	"fmt"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/awsutils"
	"github.com/kubernetes-incubator/kube-aws/node"
	nodepool "github.com/kubernetes-incubator/kube-aws/core/nodepool/config"
	"github.com/kubernetes-incubator/kube-aws/model"
)

type AmazonVPC struct {
	Enabled bool `yaml:"enabled"`
}

const (
	podSecurityGroupLogicalName = "PodSecurityGroup"
	podSubnetLogicalName = "PodSubnet"
	podSecurityGroupImport = `{"Fn::ImportValue" : {"Fn::Sub" : "${NetworkStackName}-PodSecurityGroup"}}`
	podSubnetImport = `{"Fn::ImportValue" : {"Fn::Sub" : "${NetworkStackName}-PodSubnet"}}`
	podSecurityGroupEnvName = "security_group"
	podSubnetEnvName = "subnet"
)

func (a AmazonVPC) PodSecurityGroupLogicalName() string {
	return podSecurityGroupLogicalName
}

func (a AmazonVPC) PodSubnetLogicalName() string {
	return podSubnetLogicalName
}

func (a AmazonVPC) PodSubnet() model.Subnet {
	
}

func (a AmazonVPC) ControllerEnvironment(nodepool nodepool.ProvidedConfig) map[string]string {
	podSg := nodepool.PodSecurityGroup
	if podSg.HasIdentifier() {
		podSg.IDFromFn = podSecurityGroupImport
	}
	sgRef := podSg.Ref(func() string { return "NOT_FOUND" })

	podSubnet := nodepool.PodSubnet
	if podSubnet.HasIdentifier() {
		podSubnet.IDFromFn = podSubnetImport
	}
	subnetRef := podSubnet.Ref(func() string { return "NOT_FOUND" })

	return map[string]string {
		podSecurityGroupEnvName: sgRef,
		podSubnetEnvName: subnetRef,
	}
}

func (a AmazonVPC) WorkerEnvironment(nodepool nodepool.ProvidedConfig) map[string]string {
	podSg := nodepool.PodSecurityGroup
	if podSg.HasIdentifier() {
		podSg.IDFromFn = podSecurityGroupImport
	}
	sgRef := podSg.Ref(func() string { return "NOT_FOUND" })

	podSubnet := nodepool.PodSubnet
	if podSubnet.HasIdentifier() {
		podSubnet.IDFromFn = podSubnetImport
	}
	subnetRef := podSubnet.Ref(func() string { return "NOT_FOUND" })

	return map[string]string {
		podSecurityGroupEnvName: sgRef,
		podSubnetEnvName: subnetRef,
	}
}

func (a AmazonVPC) MaxPodsScript() node.UploadedFileContent {
	script := `#!/usr/bin/env bash

set -e

declare -A instance_eni_available
`

	for it, num := range awsutils.InstanceENIsAvailable {
		script = script + fmt.Sprintf(`instance_eni_available["%s"]=%d
`, it, num)
	}

	script = script + `
declare -A instance_ip_available
`
	for it, num := range awsutils.InstanceIPsAvailable {
		script = script + fmt.Sprintf(`instance_ip_available["%s"]=%d
`, it, num)
	}

	script = script + `

instance_type=$(curl http://169.254.169.254/latest/meta-data/instance-type)

enis=${instance_eni_available["$instance_type"]}

if [ "" == "$enis" ]; then
  echo "unsupported instance type: no enis_per_eni defined: $instance_type" 1>&2
  exit 1
fi

# According to https://github.com/aws/amazon-vpc-cni-k8s#eni-allocation
ips_per_eni=${instance_ip_available["$instance_type"]}

if [ "" == "$ips_per_eni" ]; then
  echo "unsupported instance type: no ips_per_eni defined: $instance_type" 1>&2
  exit 1
fi

max_pods=$(( (enis * (ips_per_eni - 1)) + 2 ))

printf $max_pods
`
	return node.NewUploadedFileContent([]byte(script))
}
