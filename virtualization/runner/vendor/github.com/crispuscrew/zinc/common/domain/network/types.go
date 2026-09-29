// Package network defines resolved, packet-preserving network policy inputs.
package network

import "github.com/crispuscrew/zinc/common/domain/schema"

const (
	Container      = "container"
	VirtualMachine = "tap"
)

// Attachment names a device in the enforcement namespace, never a host device.
// Addresses are exact assigned addresses, not networks that an app may impersonate.
type Attachment struct {
	InterfaceID string   `json:"interface_id"`
	Device      string   `json:"device"`
	MAC         string   `json:"mac"`
	Addresses   []string `json:"addresses"`
}

// Peer carries the provisioner's authoritative identity and endpoint policy.
type Peer struct {
	AppNameID  string             `json:"app_name_id"`
	Interfaces []Attachment       `json:"interfaces"`
	Policy     schema.NetworkMeta `json:"policy"`
}

type Host struct {
	Interface string   `json:"interface"`
	Device    string   `json:"device"`
	Addresses []string `json:"addresses"`
}

type Topology struct {
	Mode               string       `json:"mode"`
	Interfaces         []Attachment `json:"interfaces"`
	Peers              []Peer       `json:"peers"`
	Hosts              []Host       `json:"hosts"`
	ExternalInterfaces []string     `json:"external_interfaces"`
}

// DomainSets are approved resolver results keyed by policy owner and rule index.
// An absent result is an error, including on deny rules; it never means wildcard.
type DomainSets map[string]map[int][]string

type Resolved struct {
	Config     schema.AppConfig
	Topology   Topology
	Domains    DomainSets
	Generation string
}

func (resolved Resolved) Endpoints(owner string) []Attachment {
	if owner == resolved.Config.AppNameID {
		return resolved.Topology.Interfaces
	}
	for _, peer := range resolved.Topology.Peers {
		if peer.AppNameID == owner {
			return peer.Interfaces
		}
	}
	return nil
}
