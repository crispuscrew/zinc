WIN_ISO ?=
WIN_APP ?= zinc-demo-win
WIN_DISK ?= $(IMAGE_DIR)/$(WIN_APP).qcow2
WIN_SIZE ?= 64
WIN_MEMORY ?= 8192
WIN_VCPUS ?= 4
WIN_RES ?= 1824x1080
WIN_RESUME ?=

## windows-demo: install a guest; existing disks need explicit WIN_RESUME=1
windows-demo: build
	@test -f "$(WIN_ISO)" || { echo 'WIN_ISO must name your Windows installation ISO'; exit 1; }
	@test -x "$(ZC)" || $(MAKE) -C ../../creator build
	./bin/zvr install --disk "$(WIN_DISK)" --size "$(WIN_SIZE)" --media "$(WIN_ISO)" \
		--firmware UEFI --secure-boot --tpm --devices Compatible \
		$(if $(WIN_RES),--resolution $(WIN_RES)) $(if $(WIN_RESUME),--resume) \
		--memory "$(WIN_MEMORY)" --vcpus "$(WIN_VCPUS)"
	@digest="$$(./bin/zvr pin "$(WIN_DISK)")"; \
	$(ZC) new "$(WIN_APP)" --vm --image "$(WIN_DISK)" --base-digest "$$digest" \
		--memory "$(WIN_MEMORY)" --vcpus "$(WIN_VCPUS)" --display Compatible --devices Compatible \
		--firmware UEFI --secure-boot --tpm --disable-gpu --cloud-init=false \
		$(if $(WIN_RES),--resolution $(WIN_RES)) --desc 'Installed Windows guest'
	@echo 'Guest created without network grants. Add reviewed interfaces and rules explicitly.'
	@echo 'Secure Boot refuses untrusted install-side NVRAM; explicit trusted-state provisioning is required before app boot.'
	@echo 'Attach a driver disc with: zvr run $(WIN_APP) --media /absolute/virtio-win.iso'

.PHONY: windows-demo
