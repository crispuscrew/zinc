.PHONY: help test test-vm lint

help:
	@printf '%s\n' 'test: run the real rootless provisioning lifecycle (build zcr first)' \
	  'test-vm: run the real rootless provisioned TAP lifecycle (build zvr first)' \
	  'lint: syntax-check the dependency-free Python harness'

test:
	PYTHONDONTWRITEBYTECODE=1 python3 -B run.py

test-vm:
	PYTHONDONTWRITEBYTECODE=1 python3 -B vm_run.py

lint:
	python3 -B -c 'import ast, pathlib; [ast.parse(path.read_text(), str(path)) for path in pathlib.Path(".").glob("*.py")]; print("Python syntax: PASS")'
