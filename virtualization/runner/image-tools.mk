VIRTIO_WIN := $(IMAGE_DIR)/virtio-win.iso
VIRTIO_URL := https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso
VIRTIO_DIGEST ?=

## virtio-win: fetch the optional driver ISO, requiring an independently authorized digest
virtio-win:
	@test -n "$(VIRTIO_DIGEST)" || { echo 'Set VIRTIO_DIGEST=sha256:... from a trusted source'; exit 1; }
	@mkdir -p "$(IMAGE_DIR)"
	@if [ ! -f "$(VIRTIO_WIN)" ]; then \
		curl -fSL -o "$(VIRTIO_WIN).part" "$(VIRTIO_URL)" && \
		test "sha256:$$(sha256sum "$(VIRTIO_WIN).part" | cut -d' ' -f1)" = "$(VIRTIO_DIGEST)" && \
		mv "$(VIRTIO_WIN).part" "$(VIRTIO_WIN)"; \
	fi
	$(MAKE) check-virtio-win

## check-virtio-win: verify the driver ISO against its independently supplied pin
check-virtio-win:
	@test -n "$(VIRTIO_DIGEST)" || { echo 'Set VIRTIO_DIGEST=sha256:...'; exit 1; }
	@test "sha256:$$(sha256sum "$(VIRTIO_WIN)" | cut -d' ' -f1)" = "$(VIRTIO_DIGEST)" || { echo 'Driver ISO pin mismatch'; exit 1; }
	xorriso -indev "$(VIRTIO_WIN)" -toc

.PHONY: virtio-win check-virtio-win
