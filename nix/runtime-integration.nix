{ pkgs, version, src }:
pkgs.runCommand "zinc-runtime-integration-${version}" {
  inherit src;
  meta = with pkgs.lib; {
    description = "Zinc runtime policies, DNS and network manifest examples, and deployment documentation";
    homepage = "https://github.com/crispuscrew/zinc";
    license = licenses.asl20;
    platforms = platforms.linux;
  };
} ''
  # Preserve repository-relative documentation links under share/zinc. These are
  # deployment inputs only; installing this package activates no host policy.
  destination="$out/share/zinc"
  mkdir -p "$destination/common/adapters/network" "$destination/common/domain/schema" \
    "$destination/common/examples" "$destination/scr" "$destination/integration/wireplumber" \
    "$destination/integration/dns"
  cp -R "$src/docs" "$destination/docs"
  cp "$src/LICENSE" "$destination/LICENSE"
  cp "$src/scr/zinc-net-install.sh" "$destination/scr/"
  for document in README.md CHANGELOG.md CONTRIBUTING.md RELEASES.md ROADMAP.md; do
    cp "$src/$document" "$destination/$document"
  done
  cp "$src/common/domain/schema/schema.go" "$destination/common/domain/schema/schema.go"
  for reference in Containerfile check.mk tool.mk go.work common/go.mod \
    creator/README.md container/runner/README.md container/e2e/README.md \
    virtualization/runner/README.md virtualization/e2e/README.md \
    launcher/gui/README.md launcher/tui/README.md launcher/demo/README.md \
    menu/README.md menu/menu.go menu/layershell.go \
    common/domain/vmoptions/config.go common/adapters/audio/connection.go \
    common/adapters/dnsproxy/lookup.go common/adapters/dnsproxy/resolver.go \
    common/adapters/dnsproxy/ready.go common/adapters/dnsproxy/serve.go \
    common/adapters/dnsproxy/cli.go virtualization/runner/domain/qemu/audio.go \
    integration/wireplumber/Makefile; do
    mkdir -p "$destination/$(dirname "$reference")"
    cp "$src/$reference" "$destination/$reference"
  done
  cp -R "$src/menu/example" "$destination/menu/example"
  cp -R "$src/menu/internal" "$destination/menu/internal"
  mkdir -p "$destination/launcher/common"
  for reference in Makefile go.mod go.sum; do
    cp "$src/launcher/common/$reference" "$destination/launcher/common/$reference"
  done
  for component in match runner store; do
    cp -R "$src/launcher/common/$component" "$destination/launcher/common/$component"
  done
  cp "$src/common/adapters/network/README.md" \
    "$src/common/adapters/network/manifest.go" "$destination/common/adapters/network/"
  cp "$src/common/examples/README.md" "$destination/common/examples/"
  for examples in apps dns runtime; do
    cp -R "$src/common/examples/$examples" "$destination/common/examples/$examples"
  done
  cp "$src/integration/wireplumber/90-zinc-audio.conf" \
    "$src/integration/wireplumber/README.md" "$src/integration/wireplumber/PROTOCOL.md" \
    "$destination/integration/wireplumber/"
  cp -R "$src/integration/wireplumber/scripts" "$destination/integration/wireplumber/scripts"
  cp "$src/integration/dns/README.md" "$src/integration/dns/sample.json" \
    "$destination/integration/dns/"
''
