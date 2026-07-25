# menu - the reusable filterable-overlay-menu UI core (pure-Go Wayland layer-shell plus a
# software renderer). It is a library, not a tool, so it includes only the containerized
# checks (check.mk); tool.mk's binary build/repro targets do not apply. Run from here:
# `make check` (and `make vendor` after changing deps).

include ../check.mk

## wallpaper-demo: generate sample images, build the wallpaper example, and open the thumbnail grid
# The counterpart to the launchers' `demo` targets, for the grid layout. Everything lands in
# bin/ (gitignored) and the samples are generated rather than committed, so the repo carries no
# binary assets. Point it at your own images with WALLPAPER_DIR=~/Pictures/Wallpapers.
DEMO_WALLS  := bin/demo-wallpapers
DEMO_BIN    := bin/wallpaper
WALLPAPER_DIR ?= $(CURDIR)/$(DEMO_WALLS)

wallpaper-demo:
	@mkdir -p bin
	@$(GO_RUN) sh -c 'go run ./example/wallpaper/gen -out /src/$(DEMO_WALLS)'
	@$(GO_RUN) sh -c 'CGO_ENABLED=0 go build -trimpath -o /src/$(DEMO_BIN) ./example/wallpaper'
	@echo "opening the wallpaper grid over $(WALLPAPER_DIR) (esc quits; enter prints the path)"
	@WALLPAPER_DIR="$(WALLPAPER_DIR)" ./$(DEMO_BIN)

.PHONY: wallpaper-demo
