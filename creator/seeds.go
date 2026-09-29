package main

// Seeds are decoded as authored YAML, just like a user's definition.
var seedApps = []struct{ name, yaml string }{
	{"example-shell", `SchemaVersion: 4
Type: ZincContainer
AppNameID: example-shell
LauncherMeta:
  Description: A terminal in a container
  Group: examples
ImageMeta:
  Image: docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
DisplayMeta:
  DisableGpuAccess: true
StartConditions:
  Entrypoint: /bin/sh
  Terminal: true
`},
	{"example-egress", `SchemaVersion: 4
Type: ZincContainer
AppNameID: example-egress
LauncherMeta:
  Description: HTTPS to example.com; other connections denied
  Group: examples
ImageMeta:
  Image: docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
DisplayMeta:
  DisableGpuAccess: true
StartConditions:
  Entrypoint: /bin/sh
  Terminal: true
NetworkMeta:
  Interfaces:
    - ID: primary
  RulesByPriority:
    - From:
        Type: Self
        Interface: primary
      To:
        Type: Internet
        Filter:
          Ports: [443]
      Domains: [example.com]
      Protocols: [TCP]
  DNS:
    ResolversByPriority:
      - Protocol: UDP
        Endpoint: "9.9.9.9:53"
`},
	{"example-instanced", `SchemaVersion: 4
Type: ZincContainer
AppNameID: example-instanced
LauncherMeta:
  Description: Run separate copies as example-instanced@work and example-instanced@personal
  Group: examples
ImageMeta:
  Image: docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
DisplayMeta:
  DisableGpuAccess: true
StartConditions:
  Entrypoint: /bin/sh
  Terminal: true
`},
}
