#!/bin/sh
# The consumer: over the private sibling link, probe the producer's published port (5432,
# should answer) and an unpublished one (9999, the producer's firewall drops it). The
# producer is reachable by its app name, which podman resolves as a network alias. Print
# one line the harness asserts on, then stay alive so its logs can be read.
probe() { echo PING | nc -w3 producer "$1" 2>/dev/null; }

reply=""
for _ in 1 2 3 4 5; do
	reply="$(probe 5432)"
	[ -n "$reply" ] && break
	sleep 1
done
[ -n "$reply" ] && published=open || published=closed

[ -n "$(probe 9999)" ] && unpublished=open || unpublished=closed

echo "PROBE 5432=$published 9999=$unpublished"
exec sleep 300
