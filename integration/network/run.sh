#!/bin/sh
set -eu
mode=${1:-test}
case "$mode" in test|lint) ;; *) echo 'usage: run.sh test|lint' >&2; exit 2;; esac
for tool in podman ip df mktemp diff; do
    command -v "$tool" >/dev/null || { echo "required tool unavailable: $tool" >&2; exit 2; }
done
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
: "${GO_IMAGE:?set the digest-pinned compiler image}"
: "${HELPER_IMAGE:?set an immutable nft helper image}"
for image in "$GO_IMAGE" "$HELPER_IMAGE"; do
    case "$image" in sha256:*|*@sha256:*) ;; *) echo "unpinned image: $image" >&2; exit 2;; esac
    if ! podman image exists "$image"; then
        echo "required pinned image unavailable: $image (no automatic pull/build)" >&2
        exit 2
    fi
done
test "$(podman info --format '{{.Host.Security.Rootless}}')" = true || {
    echo 'rootless Podman is required' >&2; exit 2;
}
storage=$(podman info --format '{{.Store.GraphRoot}}')
temporary=${TMPDIR:-/tmp}
df -Pk "$storage" "$temporary"
for location in "$storage" "$temporary"; do
    available=$(df -Pk "$location" | awk 'NR == 2 { print $4 }')
    test "$available" -ge 1048576 || { echo "need 1 GiB free at $location" >&2; exit 2; }
done
work=$(mktemp -d "$temporary/zinc-network.XXXXXX")
name=zinc-network-${work##*.}
cleanup() {
    status=$?
    trap - EXIT HUP INT TERM
    podman rm -f --ignore "$name" >/dev/null 2>&1 || status=1
    rm -rf -- "$work"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
podman run --rm --pull=never --network=none --cap-drop=ALL \
    --security-opt=no-new-privileges --security-opt=label=disable --userns=keep-id \
    --tmpfs /tmp:rw,nosuid,nodev -e GOWORK=off -e GOTOOLCHAIN=local -e GOPROXY=off \
    -e CGO_ENABLED=0 -e GOCACHE=/tmp/cache -e GOPATH=/tmp/go \
    -v "$root/common":/common:ro -v "$root/integration/network":/suite:ro \
    -v "$work":/output -w /tmp "$GO_IMAGE" sh /suite/build.sh "$mode"
test "$mode" = test || exit 0
host_namespace=$(readlink /proc/self/ns/net)
ip -o link show > "$work/host-links-before"
"$work/network.test" --host-inventory > "$work/host-inventory-before"
echo 'WARNING: THIS WILL create test-owned namespaces, links, routes and nft rules only inside a disconnected rootless container. Cleanup removes all test resources.'
echo "helper=$HELPER_IMAGE compiler=$GO_IMAGE"
echo "test selection=${TEST_PATTERN:-.}"
status=0
podman run --rm --name "$name" --pull=never --network=none --read-only \
    --sysctl net.ipv4.ip_forward=1 --sysctl net.ipv6.conf.all.forwarding=1 \
    --cap-drop=ALL --cap-add=SYS_ADMIN --cap-add=NET_ADMIN --cap-add=NET_RAW \
    --security-opt=no-new-privileges --security-opt=label=disable \
    --tmpfs /run:rw,nosuid,nodev --tmpfs /tmp:rw,nosuid,nodev \
    -e ZINC_LIVE_NETWORK=1 -e "ZINC_HOST_NETNS=$host_namespace" \
    -v "$work/network.test":/checks/network.test:ro "$HELPER_IMAGE" \
    /checks/network.test -test.v -test.timeout=240s -test.run="${TEST_PATTERN:-.}" || status=$?
ip -o link show > "$work/host-links-after"
"$work/network.test" --host-inventory > "$work/host-inventory-after"
if ! diff -u "$work/host-links-before" "$work/host-links-after"; then
    echo 'Host runtime link metadata changed during the run; checking permanent identities and routes below.'
fi
diff -u "$work/host-inventory-before" "$work/host-inventory-after" || status=1
test "$(readlink /proc/self/ns/net)" = "$host_namespace" || status=1
echo "live suite exit=$status; permanent host interfaces and IPv4/IPv6 all-table routes compared"
exit "$status"
