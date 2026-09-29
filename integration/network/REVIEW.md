# Runtime startup findings

## Fixed: preflight failure preserved old network grants

Production locations:

- `common/adapters/network/scripts.go`: initializer, supervisor and teardown.
- `common/adapters/network/namespaces.go`: namespace identity and device checks.

`TestLivePreflightFailureClosesPolicy` installs a valid permissive policy in an
isolated namespace and proves TCP echo works. It then uses the same pinned
namespace identities with `Device: missing0` and invokes the real ApplyScript.
The command fails with `Device "missing0" does not exist.` A fresh TCP connection
still completes afterward; the old `both endpoints authorized` counter increases
from one admission to two, while the default-drop counter stays zero.

This contradicts ApplyScript's documented failed-probe/default-deny guarantee.
It does not prove that a failed launch starts a new application: the command
correctly returns an error. It proves the dedicated namespace retains stale
permissions after a failed reconfiguration/preparation attempt.

Namespace identity checks are now separate from device checks. Initialization
and supervision verify the namespace, install deny-all, then check devices and
load full policy. Teardown requires only identity checks before deny-all so a
missing interface cannot preserve stale grants. A failed identity check never
modifies the entered namespace. The live regression remains enabled.

## Observed tooling boundary

The pinned minimal helper supplies BusyBox setpriv, which lacks `--bounding-set`.
The VM RunScript requires that option. VM execution on a host with util-linux
setpriv may work; this helper cannot serve as evidence that the complete VM
supervisor/capability-drop path works. Check the actual executable's supported
options in runtime prerequisites rather than checking its name alone.

The packet suite uses ApplyScript, which has no setpriv dependency. No production
file was changed as part of this integration-test work.
