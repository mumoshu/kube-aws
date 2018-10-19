package derived

import (
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
)

type EtcdCluster struct {
	clusterapi.EtcdCluster
	Network
	region    clusterapi.Region
	nodeCount int
}

func NewEtcdCluster(config clusterapi.EtcdCluster, region clusterapi.Region, network Network, nodeCount int) EtcdCluster {
	return EtcdCluster{
		EtcdCluster: config,
		region:      region,
		Network:     network,
		nodeCount:   nodeCount,
	}
}

func (c EtcdCluster) Region() clusterapi.Region {
	return c.region
}

func (c EtcdCluster) NodeCount() int {
	return c.nodeCount
}

func (c EtcdCluster) DNSNames() []string {
	var dnsName string
	if c.GetMemberIdentityProvider() == clusterapi.MemberIdentityProviderEIP {
		// Used when `etcd.memberIdentityProvider` is set to "eip"
		dnsName = fmt.Sprintf("*.%s", c.region.PublicComputeDomainName())
	}
	if c.GetMemberIdentityProvider() == clusterapi.MemberIdentityProviderENI {
		if c.InternalDomainName != "" {
			// Used when `etcd.memberIdentityProvider` is set to "eni" with non-empty `etcd.internalDomainName`
			dnsName = fmt.Sprintf("*.%s", c.InternalDomainName)
		} else {
			dnsName = fmt.Sprintf("*.%s", c.region.PrivateDomainName())
		}
	}
	return []string{dnsName}
}
