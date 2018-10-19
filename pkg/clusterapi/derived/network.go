package derived

import (
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
)

type Network interface {
	Subnets() []clusterapi.Subnet
	NATGateways() []clusterapi.NATGateway
	NATGatewayForSubnet(clusterapi.Subnet) (*clusterapi.NATGateway, error)
}

type networkImpl struct {
	subnets     []clusterapi.Subnet
	natGateways []clusterapi.NATGateway
}

func NewNetwork(subnets []clusterapi.Subnet, natGateways []clusterapi.NATGateway) Network {
	return networkImpl{
		subnets:     subnets,
		natGateways: natGateways,
	}
}

func (n networkImpl) Subnets() []clusterapi.Subnet {
	return n.subnets
}

func (n networkImpl) NATGateways() []clusterapi.NATGateway {
	return n.natGateways
}

func (n networkImpl) NATGatewayForSubnet(s clusterapi.Subnet) (*clusterapi.NATGateway, error) {
	for _, ngw := range n.NATGateways() {
		if ngw.IsConnectedToPrivateSubnet(s) {
			return &ngw, nil
		}
	}
	return nil, fmt.Errorf(`subnet "%s" doesn't have a corresponding nat gateway in: %v`, s.LogicalName(), n.natGateways)
}
