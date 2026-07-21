# zlg - Zinc's launcher (GUI): a graphical picker over the defined apps that shells out to
# the zcr binary to run them. Build logic is shared with the other tools via ../../tool.mk.
# Run from this directory: `make <target>`.
#
# NOTE: the gio front-end (internal/ui, added next) needs cgo and the Wayland/EGL/xkbcommon
# system libraries, so once it lands zlg overrides GO_IMAGE / CONTAINERFILE here to a
# graphics-enabled build image (the other tools keep the minimal one). The toolkit-neutral
# picker view-model (internal/picker) builds and tests in the standard image.

TOOL := zlg

include ../../tool.mk
