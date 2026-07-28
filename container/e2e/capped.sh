#!/bin/sh
# The containment scenario: report what the kernel actually granted this container, then
# block so the test can read the logs before stopping it.
#
# Reading cgroup files rather than trusting the argv is the whole point. The runtime's unit
# tests already prove the flags are emitted; only the kernel can say whether they took
# effect, and rootless podman needs cgroup v2 delegation for that to be true. A host without
# it would silently give an app no limits at all.
echo "UID=$(id -u)"
echo "MEMORY_MAX=$(cat /sys/fs/cgroup/memory.max 2>/dev/null || echo unreadable)"
echo "SWAP_MAX=$(cat /sys/fs/cgroup/memory.swap.max 2>/dev/null || echo unreadable)"
echo "PIDS_MAX=$(cat /sys/fs/cgroup/pids.max 2>/dev/null || echo unreadable)"
echo "capped up"
exec sleep 300
