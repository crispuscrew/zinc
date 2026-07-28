#!/bin/sh
# The readiness scenario: a dependency whose container is up long before its service is.
# This is the shape of a VPN container - podman reports it running the moment the process
# starts, while the tunnel is still handshaking, and anything routed through it in that
# window has a default route to a gateway that cannot forward yet.
#
# The delay is the point of the fixture; the file it finally touches is what the app's
# ReadyCheck looks for.
echo "slowdep starting"
sleep 5
: > /run/ready
echo "slowdep ready"
exec sleep 300
