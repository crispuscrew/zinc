#!/bin/sh
# Make podman on a CI runner able to actually start a container, and say what it settled on.
#
# Every podman-backed job calls this before its first `make`. Two things can be wrong with a
# runner's podman, and both have cost us a red build on code that was fine:
#
#   1. podman is missing entirely. Install it.
#   2. podman is present but paired with an OCI runtime too old for the spec version podman
#      emits. The current image ships podman 5.8.4 with a crun that rejects it, so `podman
#      run` dies with "crun: unknown version specified" and make sees exit 126 - a failure
#      before a single byte of Go ran. runc is also on the image and does understand it.
#
# The runtime override goes in containers.conf, not a make flag, because the e2e suite runs
# the real zcr binary and that shells out to podman itself. A CONTAINER_TOOL override would
# fix check.mk and leave the binaries under test still broken.
#
# The probe uses the same digest-pinned image the build uses, read out of check.mk so the
# digest lives in one place, so its pull is the one make would do anyway rather than extra work.
set -eu

cd "$(dirname "$0")/../.."

podman --version || { sudo apt-get update && sudo apt-get install -y podman; }

image="$(sed -n 's/^GO_IMAGE *?*= *//p' check.mk | head -1)"
[ -n "$image" ] || { echo "ensure-podman: no GO_IMAGE in check.mk" >&2; exit 1; }

if podman run --rm "$image" true 2>/dev/null; then
	echo "ensure-podman: default runtime works"
else
	echo "ensure-podman: default runtime cannot start a container, trying runc"
	command -v runc >/dev/null || { echo "ensure-podman: no runc to fall back to" >&2; exit 1; }
	mkdir -p "$HOME/.config/containers"
	printf '[engine]\nruntime = "runc"\n' >"$HOME/.config/containers/containers.conf"
	podman run --rm "$image" true
	echo "ensure-podman: switched to runc"
fi

# Printed unconditionally: when a build goes red on runner skew rather than on our code, this
# is the line that says so.
podman info --format 'podman {{.Version.Version}}, runtime {{.Host.OCIRuntime.Name}} {{.Host.OCIRuntime.Version}}'
