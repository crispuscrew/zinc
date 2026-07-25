# zlt - Zinc's launcher (TUI): a fast fuzzy picker over the defined apps that shells out
# to the zcr binary to run them. Build logic is shared with zcc/zcr via ../../tool.mk.
# Run from this directory: `make <target>`.

TOOL := zlt

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
