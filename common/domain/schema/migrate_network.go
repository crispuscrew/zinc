package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (change *migration) migrateNetwork() error {
	lists := change.value("NetworkMeta.NetworkLists")
	if lists != nil {
		if change.value("NetworkMeta.RulesByPriority") != nil {
			return fmt.Errorf("NetworkMeta.NetworkLists and RulesByPriority are both set; rewrite the rules explicitly")
		}
		if lists.Tag != "!!null" && lists.Kind != yaml.SequenceNode {
			return fmt.Errorf("NetworkMeta.NetworkLists must be a list")
		}
		for index, list := range lists.Content {
			rule, err := migrateNetworkList(list)
			if err != nil {
				return fmt.Errorf("NetworkMeta.NetworkLists[%d]: %w", index, err)
			}
			lists.Content[index] = rule
		}
		if err := change.move("NetworkMeta.NetworkLists", "NetworkMeta.RulesByPriority"); err != nil {
			return err
		}
	}
	return change.migrateDNS()
}

func migrateNetworkList(list *yaml.Node) (*yaml.Node, error) {
	if list.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping; rewrite using explicit From and To peers")
	}
	// Old routing and scoped links also created topology, which a packet rule alone
	// cannot reproduce. Blacklist-only chains had an implicit allow default.
	for _, field := range [][2]string{
		{"Host", "bool"}, {"AppName", "string"}, {"Interface", "string"},
		{"Ingress", "bool"}, {"Blacklist", "bool"}, {"Via", "bool"},
		{"Forward", "bool"}, {"ForwardPorts", "list"},
		{"GatewayV4", "string"}, {"GatewayV6", "string"},
	} {
		if value := mappingValue(list, field[0]); value != nil && !legacyZero(value, field[1]) {
			return nil, fmt.Errorf("%s cannot be migrated losslessly; specify Interfaces and RulesByPriority with explicit peers, routing and default-deny policy", field[0])
		}
	}
	filter := mappingNode()
	rule := mappingNode()
	addresses := false
	ports := false
	for index := 0; index < len(list.Content); index += 2 {
		key, value := list.Content[index], list.Content[index+1]
		switch key.Value {
		case "IPv4CIDR", "IPv6CIDR", "Ports", "Domains":
			if value.Tag != "!!null" && value.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("%s must be a list", key.Value)
			}
			if key.Value == "Domains" {
				rule.Content = append(rule.Content, key, value)
			} else {
				filter.Content = append(filter.Content, key, value)
			}
			if key.Value == "Ports" {
				ports = len(value.Content) > 0
			} else {
				addresses = addresses || len(value.Content) > 0
			}
		case "Host", "AppName", "Interface", "Ingress", "Blacklist", "Via", "Forward", "ForwardPorts", "GatewayV4", "GatewayV6":
		default:
			return nil, fmt.Errorf("unknown legacy field %s; rewrite using RulesByPriority", key.Value)
		}
	}
	if !addresses {
		return nil, fmt.Errorf("an old list without addresses/domains has no lossless rule equivalent; state the intended destination explicitly")
	}
	from := mappingNode()
	from.Content = []*yaml.Node{scalarNode("Type"), scalarNode(string(NetworkPeerSelf))}
	to := mappingNode()
	to.Content = []*yaml.Node{scalarNode("Type"), scalarNode(string(NetworkPeerInternet)), scalarNode("Filter"), filter}
	rule.Content = append(rule.Content, scalarNode("From"), from, scalarNode("To"), to)
	if ports {
		protocols, err := encodedNode([]NetworkProtocol{NetworkTCP, NetworkUDP})
		if err != nil {
			return nil, err
		}
		rule.Content = append(rule.Content, scalarNode("Protocols"), protocols)
	}
	return rule, nil
}
