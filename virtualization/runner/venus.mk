VIRGL_VERSION ?= 1.3.0
VIRGL_COMMIT ?= ca50e008863837e094747a69974dde3ae148aeaa
VIRGL_PREFIX ?= $(HOME)/.local/share/zinc/virgl-venus
VIRGL_SRC ?= $(HOME)/.cache/zinc/virglrenderer

## virgl-venus: build the pinned Venus renderer in the user's own data directory
virgl-venus:
	@for tool in git meson ninja pkg-config; do command -v $$tool >/dev/null || exit 1; done
	pkg-config --exists vulkan libdrm epoxy
	@if [ ! -d "$(VIRGL_SRC)/.git" ]; then \
		mkdir -p "$$(dirname "$(VIRGL_SRC)")"; \
		git clone --depth 1 --branch "virglrenderer-$(VIRGL_VERSION)" \
			https://gitlab.freedesktop.org/virgl/virglrenderer.git "$(VIRGL_SRC)"; \
	fi
	@test "$$(git -C "$(VIRGL_SRC)" rev-parse HEAD)" = "$(VIRGL_COMMIT)" || { echo 'Renderer commit pin mismatch'; exit 1; }
	meson setup "$(VIRGL_SRC)/build" "$(VIRGL_SRC)" --prefix="$(VIRGL_PREFIX)" -Dvenus=true -Dbuildtype=release
	ninja -C "$(VIRGL_SRC)/build"
	ninja -C "$(VIRGL_SRC)/build" install
	@test -f "$(VIRGL_PREFIX)/lib64/libvirglrenderer.so.1"
	@test -x "$(VIRGL_PREFIX)/libexec/virgl_render_server"

.PHONY: virgl-venus
