# zcr — Zinc's container runtime: it runs an app file via podman, applying the egress
# lock-down before the app starts (docs/architecture.md §5.3, §9.1). The build logic is
# shared with zcc via ../../tool.mk. Run from this directory: `make <target>`.

TOOL := zcr

include ../../tool.mk

# Helper image (nft) carrying nftables for the egress/ingress lock-down init step
# (§5.3). Built from the vetted, digest-pinned Containerfile; the tag must match the
# enforcer's DefaultNetfilterImage.
NETFILTER_IMAGE ?= zinc/netfilter:local

## netfilter-image: build the nft helper image the enforcer applies rules with (§5.3)
netfilter-image:
	$(CONTAINER_TOOL) build -t $(NETFILTER_IMAGE) ../creator/images/netfilter

.PHONY: netfilter-image
