# Zinc Virtualization Runner (zvr). Build logic is shared with the other tools via
# ../../tool.mk. Run from this directory: `make <target>`.

TOOL := zvr

include ../../tool.mk

## demo: download a Fedora Cloud image, author a VM app, and boot it with an accelerated window
# The counterpart to the launchers' `demo` targets. Unlike those it needs a real guest OS, so
# it downloads one once (~600 MB) into the shared image directory and pins it by the digest
# Fedora publishes - if the file does not hash to that, the demo stops rather than booting
# something unverified.
FEDORA_IMAGE   := Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2
FEDORA_URL     := https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/$(FEDORA_IMAGE)
FEDORA_DIGEST  := sha256:846574c8a97cd2d8dc1f231062d73107cc85cbbbda56335e264a46e3a6c8ab2f

IMAGE_DIR ?= $(HOME)/.local/share/zinc/images
DEMO_APP  ?= zinc-demo-vm
DEMO_PORT ?= 2222
SSH_KEY   ?= $(firstword $(wildcard $(HOME)/.ssh/id_ed25519.pub $(HOME)/.ssh/id_rsa.pub))
ZC        ?= ../../creator/bin/zc

demo: build
	@test -n "$(SSH_KEY)" || { echo "no ssh public key in ~/.ssh; make one with: ssh-keygen -t ed25519"; exit 1; }
	@test -x "$(ZC)" || $(MAKE) -C ../../creator build
	@mkdir -p "$(IMAGE_DIR)"
	@if [ ! -f "$(IMAGE_DIR)/$(FEDORA_IMAGE)" ]; then \
		echo "downloading $(FEDORA_IMAGE) (~600 MB, once)"; \
		curl -fSL --progress-bar -o "$(IMAGE_DIR)/$(FEDORA_IMAGE).part" "$(FEDORA_URL)"; \
		mv "$(IMAGE_DIR)/$(FEDORA_IMAGE).part" "$(IMAGE_DIR)/$(FEDORA_IMAGE)"; \
	fi
	@got="$$(./bin/zvr pin "$(IMAGE_DIR)/$(FEDORA_IMAGE)")"; \
	if [ "$$got" != "$(FEDORA_DIGEST)" ]; then \
		echo "the downloaded image does not match the digest Fedora published:"; \
		echo "  published: $(FEDORA_DIGEST)"; echo "  on disk:   $$got"; exit 1; \
	fi; \
	echo "base image verified against Fedora's published digest"
	@./bin/zvr stop $(DEMO_APP) --force >/dev/null 2>&1 || true
	@$(ZC) delete $(DEMO_APP) >/dev/null 2>&1 || true
	@$(ZC) new $(DEMO_APP) --vm \
		--image "$(IMAGE_DIR)/$(FEDORA_IMAGE)" \
		--base-digest "$(FEDORA_DIGEST)" \
		--memory 4096 --vcpus 4 --disk 20 \
		--display Accelerated \
		--ci-user zinc --ci-ssh-key "$(SSH_KEY)" \
		--forward $(DEMO_PORT):22 \
		--desc "0.4 demo guest: accelerated display smoke test"
	@./bin/zvr run $(DEMO_APP)
	@echo
	@echo "A qemu window should now be open on your compositor, showing the guest boot."
	@echo "First boot installs the 3D test tools, so give it a minute or two."
	@echo
	@echo "Then check the display path is really accelerated:"
	@echo "  ssh -p $(DEMO_PORT) -o StrictHostKeyChecking=no zinc@127.0.0.1 glxinfo -B"
	@echo "Look for virgl or virtio_gpu in the renderer line. llvmpipe means the guest fell"
	@echo "back to software rendering and the accelerated path is not working."
	@echo
	@echo "  make -C virtualization/runner demo-status   is it running"
	@echo "  ./bin/zvr stop $(DEMO_APP)                  shut it down (graceful, ACPI)"
	@echo "  ./bin/zvr reset $(DEMO_APP)                 throw the disk away, back to the base"

## demo-status: what the demo guest is doing
demo-status: build
	@./bin/zvr status $(DEMO_APP)

.PHONY: demo demo-status
