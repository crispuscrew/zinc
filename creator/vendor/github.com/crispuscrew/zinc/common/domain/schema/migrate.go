package schema

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"gopkg.in/yaml.v3"
)

const previousSchemaVersion = 3

// Migrate converts recognized legacy fields before strict decoding, including
// versionless inheritance fragments. Current documents remain byte-identical.
func Migrate(data []byte) ([]byte, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return data, nil
		}
		return nil, fmt.Errorf("decode app migration: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("app migration requires exactly one YAML document")
	}
	// yaml's decoder bounds alias expansion, including deeply repeated aliases.
	var checked any
	if err := document.Decode(&checked); err != nil {
		return nil, fmt.Errorf("decode app migration: %w", err)
	}
	root, err := expandMigrationNode(document.Content[0], map[*yaml.Node]bool{})
	if err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return data, nil
	}
	versionNode := mappingValue(root, "SchemaVersion")
	var version int
	if versionNode != nil {
		if err := versionNode.Decode(&version); err != nil ||
			(version != previousSchemaVersion && version != SchemaVersion) {
			return data, nil
		}
	}
	change := migration{root: root}
	legacy := version == previousSchemaVersion || change.recognizesLegacy()
	if !legacy {
		return data, nil
	}
	for _, migrate := range []func() error{change.renameFields, change.removeResidue,
		change.migrateAudio, change.migrateNetwork, func() error { return change.migrateVM(version) }} {
		if err := migrate(); err != nil {
			return nil, fmt.Errorf("migrate app: %w", err)
		}
	}
	if version == previousSchemaVersion {
		*versionNode = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(SchemaVersion)}
		change.changed = true
	}
	if !change.changed {
		return data, nil
	}
	document.Content[0] = root
	migrated, err := yaml.Marshal(&document)
	if err != nil {
		return nil, fmt.Errorf("encode migrated app: %w", err)
	}
	return migrated, nil
}

type migration struct {
	root    *yaml.Node
	changed bool
}

func (change *migration) recognizesLegacy() bool {
	for _, names := range legacyRenames {
		if change.value(names[0]) != nil {
			return true
		}
	}
	for _, path := range []string{"VirtualizationMeta", "Capabilities", "ResourcesMeta.MaxSwapMiB",
		"StartConditions.ReadyCheck", "StartConditions.ReadyTimeoutSec", "NetworkMeta.Tunnel",
		"NetworkMeta.NetworkLists", "NetworkMeta.DNSServers"} {
		if change.value(path) != nil {
			return true
		}
	}
	for _, direction := range audioDirections {
		value := change.value("AudioMeta." + direction)
		if value != nil && value.Tag != "!!null" && value.Kind != yaml.MappingNode {
			return true
		}
	}
	return false
}
