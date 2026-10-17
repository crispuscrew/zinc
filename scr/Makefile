.PHONY: help test lint

help:
	@printf '%s\n' 'make -C scr test - run offline installer tests (Python 3, stdlib only)' \
		'make -C scr lint - check shell syntax and optional ShellCheck'

test: lint
	python3 -B install_test.py
	python3 -B install_ux_test.py

lint:
	sh -n zinc-net-install.sh
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck -s sh zinc-net-install.sh; \
	else printf '%s\n' 'ShellCheck unavailable; shell syntax passed.'; fi
