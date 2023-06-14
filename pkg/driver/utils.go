package driver

import (
	"fmt"
	"github.com/winrouter/csi-hostpath/pkg/server"
	v1 "k8s.io/api/core/v1"
	"net"
)

func getNodeAddr(node *v1.Node, nodeID string, useNodeHostname bool) (string, error) {
	if useNodeHostname {
		return node.Name + ":" + server.GetLvmdPort(), nil
	}
	ip, err := GetNodeIP(node, nodeID)
	if err != nil {
		return "", err
	}
	if ip.To4() == nil {
		// ipv6: https://stackoverflow.com/a/22752227
		return fmt.Sprintf("[%s]", ip.String()) + ":" + server.GetLvmdPort(), nil
	}
	return ip.String() + ":" + server.GetLvmdPort(), nil
}

// GetNodeIP get node address
func GetNodeIP(node *v1.Node, nodeID string) (net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[v1.NodeAddressType][]v1.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[v1.NodeInternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[v1.NodeExternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	return nil, fmt.Errorf("Node IP unknown; known addresses: %v", addresses)
}