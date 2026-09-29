package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var legacyRenames = [][2]string{
	{"Icon", "LauncherMeta.Icon"},
	{"Description", "LauncherMeta.Description"},
	{"Group", "LauncherMeta.Group"},
	{"Env", "StartConditions.EntrypointEnv"},
	{"ReadOnlyRootfs", "StartConditions.ReadOnlyRootfs"},
	{"StartConditions.Multiterminal", "StartConditions.Attached"},
	{"StartConditions.MultiterminalEntrypoint", "StartConditions.AttachedEntrypoint"},
	{"StartConditions.MultiterminalEnv", "StartConditions.AttachedEnv"},
	{"StartConditions.Autorestart", "StopConditions.Autorestart"},
}

func (change *migration) renameFields() error {
	for _, names := range legacyRenames {
		if err := change.move(names[0], names[1]); err != nil {
			return err
		}
	}
	return nil
}

func (change *migration) removeResidue() error {
	for _, field := range []struct{ path, kind string }{
		{"Capabilities", "list"}, {"ResourcesMeta.MaxSwapMiB", "number"},
		{"StartConditions.ReadyCheck", "list"}, {"StartConditions.ReadyTimeoutSec", "number"},
		{"NetworkMeta.Tunnel.WireGuardConf", "string"},
	} {
		if err := change.removeZero(field.path, field.kind); err != nil {
			return err
		}
	}
	return change.removeEmpty("NetworkMeta.Tunnel")
}

func (change *migration) removeZero(path, kind string) error {
	value := change.value(path)
	if value == nil {
		return nil
	}
	if !legacyZero(value, kind) {
		return fmt.Errorf("%s cannot be represented in schema v%d; configure separate runtime options before removing it (only zero legacy residue can be removed)", path, SchemaVersion)
	}
	change.remove(path)
	return nil
}

func legacyZero(value *yaml.Node, kind string) bool {
	if value.Tag == "!!null" {
		return true
	}
	switch kind {
	case "string":
		return value.Tag == "!!str" && value.Value == ""
	case "list":
		return value.Kind == yaml.SequenceNode && len(value.Content) == 0
	case "bool":
		var flag bool
		return value.Tag == "!!bool" && value.Decode(&flag) == nil && !flag
	case "number":
		var number float64
		return (value.Tag == "!!int" || value.Tag == "!!float") && value.Decode(&number) == nil && number == 0
	}
	return false
}

func (change *migration) removeEmpty(path string) error {
	value := change.value(path)
	if value == nil {
		return nil
	}
	if value.Tag != "!!null" && (value.Kind != yaml.MappingNode || len(value.Content) != 0) {
		return fmt.Errorf("%s has unsupported legacy settings; migrate them explicitly to separate runtime options", path)
	}
	change.remove(path)
	return nil
}
