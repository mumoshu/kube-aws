package cluster

import (
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"strings"
	"testing"
)

const cluster_config = `
availabilityZone: us-west-1c
keyName: test-key-name
region: us-west-1
clusterName: test-cluster-name
s3URI: s3://bucket/demo
kmsKeyArn: "arn:aws:kms:us-west-1:xxxxxxxxx:key/xxxxxxxxxxxxxxxxxxx"
apiEndpoints:
- name: public
  dnsName: test.staging.core-os.net
  loadBalancer:
    hostedZone:
      id: hostedzone-xxxxxx

kubelet:
  rotateCerts: 
    enabled: true
`

func ConfigFromBytes(data []byte) (*Config, error) {
	c, err := ClusterFromBytes(data)
	if err != nil {
		return nil, err
	}
	opts := clusterapi.ClusterOptions{
		S3URI: c.S3URI,
		// TODO
		SkipWait: false,
	}

	cpConfig, err := Compile(c, opts)
	if err != nil {
		return nil, err
	}

	return cpConfig, nil
}

func NodePoolConfigFromBytes(data []byte) (*NodePoolConfig, error) {
	c, err := ConfigFromBytes(data)
	if err != nil {
		return nil, err
	}

	return NodePoolCompile(c.Worker.NodePools[0], c)
}

func TestNodePoolRotateCerts(t *testing.T) {
	cpconfig, _ := ConfigFromBytes([]byte(cluster_config))

	npconfig, _ := NodePoolCompile(clusterapi.WorkerNodePool{}, cpconfig)

	if !(npconfig.FeatureGates()["RotateKubeletClientCertificate"] == "true") {
		t.Errorf("When RotateCerts is enabled, Feature Gate RotateKubeletClientCertificate should be automatically enabled too")
	}
}

func TestKube2IamKiamClash(t *testing.T) {
	config := `
name: nodepool1
kube2IamSupport:
  enabled: true
kiamSupport:
  enabled: true
`

	_, err := ConfigFromBytes([]byte(cluster_config))
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("expected config to cause error as kube2iam and kiam cannot be enabled together: %s\n%s", err, config)
	}
}

const externalDNSNameConfig = `externalDNSName: test.staging.core-os.net
`

const availabilityZoneConfig = `availabilityZone: us-west-1c
`

const apiEndpointMinimalConfigYaml = `keyName: test-key-name
region: us-west-1
s3URI: s3://mybucket/mydir
clusterName: test-cluster-name
kmsKeyArn: "arn:aws:kms:us-west-1:xxxxxxxxx:key/xxxxxxxxxxxxxxxxxxx"
`
const minimalConfigYaml = externalDNSNameConfig + apiEndpointMinimalConfigYaml
const singleAzConfigYaml = minimalConfigYaml + availabilityZoneConfig

func TestRktConfig(t *testing.T) {
	validChannels := []string{
		"alpha",
		"beta",
		"stable",
	}

	conf := func(channel string) string {
		return fmt.Sprintf(`containerRuntime: rkt
releaseChannel: %s
`, channel)
	}

	for _, channel := range validChannels {
		confBody := singleAzConfigYaml + conf(channel)
		_, err := ConfigFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
		}
	}
}

func TestWithTrailingDot(t *testing.T) {
	tests := [][]string{
		[]string{
			"",
			"",
		},
		[]string{
			"foo.bar.",
			"foo.bar.",
		},
		[]string{
			"foo.bar",
			"foo.bar.",
		},
	}

	for _, test := range tests {
		input := test[0]
		actual := WithTrailingDot(input)
		expected := test[1]

		if expected != actual {
			t.Errorf(
				"WithTrailingDot(\"%s\") expected to return \"%s\" but it returned: \"%s\"",
				input,
				expected,
				actual,
			)
		}
	}
}
