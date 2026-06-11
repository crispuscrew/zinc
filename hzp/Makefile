# HyprZinc Podman (hzp). The build logic is shared with hzl/hzv via ../tool.mk
# ("the same logic, only different paths"). Run from this directory: `make <target>`.

TOOL := hzp

# Sample config for the validate/run convenience targets (under hzp/).
APP      ?= examples/apps/firefox.toml
RUN_ARGS  = run $(APP)

include ../tool.mk

## validate: validate the sample app TOML (override with APP=path)
validate:
	$(GO) run . validate $(APP)

.PHONY: validate
