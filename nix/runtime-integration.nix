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
  mkdir -p "$destination/common/adapters/network" \
    "$destination/common/examples" "$destination/integration/wireplumber" \
    "$destination/integration/dns"
  cp -R "$src/docs" "$destination/docs"
  cp "$src/LICENSE" "$destination/LICENSE"
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
