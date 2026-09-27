FEDORA_IMAGE := Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2
FEDORA_URL := https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/$(FEDORA_IMAGE)
FEDORA_DIGEST := sha256:846574c8a97cd2d8dc1f231062d73107cc85cbbbda56335e264a46e3a6c8ab2f
IMAGE_DIR ?= $(HOME)/.local/share/zinc/images
DEMO_APP ?= zinc-demo-vm
DEMO_PORT ?= 2222
SSH_KEY ?= $(firstword $(wildcard $(HOME)/.ssh/id_ed25519.pub $(HOME)/.ssh/id_rsa.pub))
ZC ?= ../../creator/bin/zc

## demo: create a pinned cloud guest with explicit network policy and separate VM options
demo: build
	@test -n "$(SSH_KEY)" || { echo 'SSH_KEY must name a public key'; exit 1; }
	@test -x "$(ZC)" || $(MAKE) -C ../../creator build
	@mkdir -p "$(IMAGE_DIR)"
	@if [ ! -f "$(IMAGE_DIR)/$(FEDORA_IMAGE)" ]; then \
		curl -fSL -o "$(IMAGE_DIR)/$(FEDORA_IMAGE).part" "$(FEDORA_URL)" && \
		mv "$(IMAGE_DIR)/$(FEDORA_IMAGE).part" "$(IMAGE_DIR)/$(FEDORA_IMAGE)"; \
	fi
	@test "$$(./bin/zvr pin "$(IMAGE_DIR)/$(FEDORA_IMAGE)")" = "$(FEDORA_DIGEST)" || { echo 'Fedora image pin mismatch'; exit 1; }
	$(ZC) new "$(DEMO_APP)" --vm --image "$(IMAGE_DIR)/$(FEDORA_IMAGE)" \
		--base-digest "$(FEDORA_DIGEST)" --memory 4096 --vcpus 4 --disk 20 \
		--display Accelerated --cloud-init --ci-user zinc --ci-ssh-key "$(SSH_KEY)" \
		--interface primary --forward "$(DEMO_PORT):22" \
		--network-rule '{"From":{"Type":"Self"},"To":{"Type":"Internet"},"Protocols":["TCP","UDP"]}' \
		--network-rule '{"From":{"Type":"Host"},"To":{"Type":"Self","Filter":{"Ports":[22]}},"Protocols":["TCP"]}' \
		--dns-resolver '{"Protocol":"UDP","Endpoint":"9.9.9.9:53"}' \
		--install 'dnf install -y mesa-demos glx-utils vulkan-tools mesa-vulkan-drivers glmark2 weston' \
		--desc 'GPU demonstration guest'
	./bin/zvr run "$(DEMO_APP)"

## demo-status: inspect the demo guest
demo-status:
	./bin/zvr status "$(DEMO_APP)"

## demo-stop: stop the demo guest
demo-stop:
	./bin/zvr stop "$(DEMO_APP)"

## demo-reset: explicitly delete the demo guest disk, NVRAM and TPM state
demo-reset:
	./bin/zvr reset "$(DEMO_APP)" --confirm

## demo-gpu-check: query guest renderers through the explicitly published SSH port
demo-gpu-check:
	ssh -p "$(DEMO_PORT)" zinc@127.0.0.1 'eglinfo -B; vulkaninfo --summary'

.PHONY: demo demo-status demo-stop demo-reset demo-gpu-check
