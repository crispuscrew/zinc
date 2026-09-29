package audio

import (
	"fmt"
	"strings"

	plan "github.com/crispuscrew/zinc/common/domain/audio"
)

func checkEndpoints(endpoints []Endpoint, token string, selections []plan.Selection) error {
	if len(endpoints) == 0 || len(endpoints) > len(selections) {
		return errProtocol
	}
	seen := map[string]bool{}
	for _, endpoint := range endpoints {
		if !strings.HasPrefix(endpoint.Name, "za."+token+".") || seen[endpoint.Name] {
			return errProtocol
		}
		matched := false
		for _, selection := range selections {
			if selection.Direction == endpoint.Direction && (selection.Default || selection.Name == endpoint.Target) {
				matched = true
			}
		}
		if !matched {
			return errProtocol
		}
		seen[endpoint.Name] = true
	}
	for _, selection := range selections {
		matched := false
		for _, endpoint := range endpoints {
			if selection.Direction == endpoint.Direction && (selection.Default || selection.Name == endpoint.Target) {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("audio: policy omitted a requested endpoint")
		}
	}
	return nil
}
