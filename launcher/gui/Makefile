# zlg - Zinc's launcher (GUI): a graphical picker over the defined apps that shells out to
# the zcr binary to run them. Build logic is shared with the other tools via ../../tool.mk.
# Run from this directory: `make <target>`.
#
# zlg renders in pure Go (it speaks the Wayland wire protocol directly via go-wayland and
# software-renders into a shared-memory buffer), so it needs no cgo and no graphics
# libraries: it builds static / CGO=0 from the same minimal image as the other tools, with
# no GO_IMAGE / CONTAINERFILE override.

TOOL := zlg

include ../../tool.mk

## demo: build, then open the picker against the bundled example apps in a throwaway config dir (your real config is untouched)
DEMO_HOME := $(BIN_DIR)/demo-home
demo: build
	@rm -rf "$(DEMO_HOME)/zinc/apps"
	@mkdir -p "$(DEMO_HOME)/zinc/apps"
	@cp $(REPO_REL)/launcher/demo/apps/*.yaml "$(DEMO_HOME)/zinc/apps/"
	@echo "launching $(TOOL) against the demo apps (your real ~/.config/zinc is untouched)"
	@XDG_CONFIG_HOME="$(CURDIR)/$(DEMO_HOME)" ./$(BIN)

.PHONY: demo
