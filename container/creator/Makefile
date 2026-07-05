# HyprZinc Container (hzc). The build logic is shared with hzl/hzv via ../tool.mk
# ("the same logic, only different paths"). Run from this directory: `make <target>`.

TOOL := hzc

# Sample config for the validate/run convenience targets (under hzc/).
APP      ?= examples/apps/firefox.toml
RUN_ARGS  = run $(APP)

include ../tool.mk

# Helper image (nft) for the pasta egress lock-down init step (§5.3).
NETFILTER_IMAGE ?= hyprzinc/trusted-netfilter:local

## validate: build in the pinned container, then validate the sample app TOML (override with APP=path)
validate: build
	./$(BIN) validate $(APP)

## netfilter-image: build the nft helper image for pasta egress enforcement (§5.3)
netfilter-image:
	$(CONTAINER_TOOL) build -t $(NETFILTER_IMAGE) images/netfilter

.PHONY: validate netfilter-image
