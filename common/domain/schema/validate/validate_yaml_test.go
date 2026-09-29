package validate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

// Exercise the actual authoring format through decoding and validation, including
// endpoint filters whose source/destination placement must survive the wire format.
func TestSharedVMYAMLValidation(test *testing.T) {
	document := fmt.Sprintf(`SchemaVersion: %d
Type: ZincVirtualization
AppNameID: guest
StartConditions:
  DependsOn: [database]
  SecureBoot: true
  TPM: true
StopConditions:
  Background: true
  Autorestart: true
ResourcesMeta:
  MaxCPUCores: 4
  MaxRamMiB: 4096
ImageMeta:
  Image: /var/lib/zinc/base.qcow2
  CloudInit: true
  PublicSSHKeyPath: /home/user/key.pub
  Install: ["dnf install -y app"]
InternalUserMeta:
  UseNonRootUser: true
  NonRootUserName: guest
DisplayMeta:
  DisplayWidth: 1920
  DisplayHeight: 1080
  DisableGpuAccess: true
NetworkMeta:
  Interfaces:
    - ID: wan
      MacAddress: 02:11:22:33:44:55
  RulesByPriority:
    - From:
        Type: Self
        Interface: wan
        Filter:
          Ports: [1024]
      To:
        Type: Internet
        Filter:
          IPv4CIDR: [0.0.0.0/0]
          Ports: [443]
      Domains: [example.com]
      Protocols: [TCP, UDP, SCTP]
      AllowAllExcept: false
    - From: {Type: App, AppNameID: database, Interface: internal}
      To: {Type: Self, Interface: wan}
      AllowAllExcept: true
  DNS:
    ResolversByPriority:
      - Protocol: HTTPS
        Endpoint: dns.example.com:443
        Path: /dns-query
        BootstrapIPs: [1.1.1.1, "2606:4700:4700::1111"]
AudioMeta:
  Playback:
    PipeWireDefault: true
    PipeWireDevices: [alsa_output.card]
    ALSADevices: [/dev/snd/controlC0, /dev/snd/pcmC0D0p]
  Monitor:
    PipeWireDevices: [alsa_output.card]
CreatorFlags: ["--option=value with spaces"]
RunnerFlags: ["-name", "custom guest"]
`, schema.SchemaVersion)
	decode := func(document string) schema.AppConfig {
		test.Helper()
		var cfg schema.AppConfig
		decoder := yaml.NewDecoder(strings.NewReader(document))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			test.Fatal(err)
		}
		return cfg
	}
	if err := Validate(decode(document)); err != nil {
		test.Fatal(err)
	}
	malformed := strings.Replace(document, "Ports: [1024]", "Ports: [65536]", 1)
	requireError(test, decode(malformed), "From.Filter.Ports")
	malformed = strings.Replace(document, "Ports: [443]", "Ports: [65536]", 1)
	requireError(test, decode(malformed), "To.Filter.Ports")
}
