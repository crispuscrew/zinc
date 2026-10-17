#!/bin/sh
# Self-contained so one downloaded script handles installation and the optional demo.
version=v0.11.1
bin_dir=
example=1
tools='zc zcr zlg zlt zvr'
work_dir='' stage_dir='' sample_stage='' published=''

fail() {
	printf 'zinc-net-install.sh: %s\n' "$*" >&2
	[ -z "$published" ] || printf 'Already replaced:%s; no cross-tool rollback.\n' "$published" >&2
	exit 1
}
usage() {
	printf '%s\n' 'Usage: sh zinc-net-install.sh [--version X.Y.Z|vX.Y.Z] [--bin-dir DIR] [--no-example] [--help]' \
		'Install zc, zcr, zlg, zlt and zvr; all are verified before atomic per-file replacement.' \
		"Defaults: $version and \$HOME/.local/bin. Requires Linux AMD64, curl and standard Linux utilities."
}
while [ "$#" -gt 0 ]; do
	case "$1" in
		--help) usage; exit 0 ;;
		--no-example) example=0; shift ;;
		--version) [ "$#" -ge 2 ] || fail 'invalid --version argument'
			version=$2; shift 2 ;;
		--bin-dir) [ "$#" -ge 2 ] || fail 'invalid --bin-dir argument'
			case "$2" in ''|--*) fail 'invalid --bin-dir argument' ;; esac
			bin_dir=$2; shift 2 ;;
		*) fail "unknown argument: $1" ;;
	esac
done
for utility in awk curl uname sha256sum mktemp mkdir cp chmod stat mv rm sed grep cat ln dirname tail; do
	command -v "$utility" >/dev/null 2>&1 || fail "required utility missing: $utility"
done
version=${version#v}
printf '%s\n' "$version" | LC_ALL=C awk '
	NR != 1 || !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { exit 1 }
	END { if (NR != 1) exit 1 }' || fail 'version must be stable X.Y.Z or vX.Y.Z (no leading zeros)'
version=v$version
if [ -z "$bin_dir" ]; then
	case "${HOME:-}" in /*) ;; *) fail 'HOME must be an absolute directory; or use --bin-dir' ;; esac
	[ -d "$HOME" ] || fail 'HOME directory does not exist'
	bin_dir=$HOME/.local/bin
fi
system=$(uname -s) || fail 'cannot detect operating system'
machine=$(uname -m) || fail 'cannot detect architecture'
case "$system:$machine" in Linux:x86_64|Linux:amd64) ;; *) fail "unsupported platform: $system $machine" ;; esac
case "$bin_dir" in /*) ;; *) bin_dir=$(pwd -P)/$bin_dir || fail 'cannot resolve working directory' ;; esac
case "$bin_dir" in *:*|*'
'*) fail 'bin directory must not contain colons or newlines' ;; esac
check_targets() {
	[ ! -L "$bin_dir" ] || fail "bin directory is a symlink: $bin_dir"
	[ ! -e "$bin_dir" ] || { [ -d "$bin_dir" ] && [ -w "$bin_dir" ] && [ -x "$bin_dir" ]; } ||
		fail "bin directory must be writable/searchable: $bin_dir"
	for tool in $tools; do
		target=$bin_dir/$tool
		[ ! -L "$target" ] || fail "destination is a symlink: $target"
		[ ! -e "$target" ] || { [ -f "$target" ] && [ -w "$target" ] && [ -x "$target" ]; } ||
			fail "destination must be a writable executable regular file: $target"
	done
}
check_targets
cleanup() {
	result=$?; trap - 0
	for scratch in "$sample_stage" "$stage_dir" "$work_dir"; do
		[ -z "$scratch" ] || rm -rf -- "$scratch" || { printf 'Cannot clean: %s\n' "$scratch" >&2; result=1; }
	done
	exit "$result"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/zinc-install.XXXXXX") || fail 'cannot create download directory'
release_url=https://github.com/crispuscrew/zinc/releases/download/$version
for asset in zc-linux-amd64 zcr-linux-amd64 zlg-linux-amd64 zlt-linux-amd64 zvr-linux-amd64 SHA256SUMS; do
	curl -q --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
		--output "$work_dir/$asset" "$release_url/$asset" || fail "download failed: $asset"
done
LC_ALL=C awk '
	{ hash = substr($0, 1, 64); name = substr($0, 67)
		if (length(hash) != 64 || hash !~ /^[0-9a-fA-F]+$/ || substr($0, 65, 2) != "  " ||
		    name !~ /^(zc|zcr|zlg|zlt|zvr)-linux-amd64$/ || seen[name]++) exit 1
		print tolower(hash) "  " name }
	END { if (NR != 5) exit 1 }' < "$work_dir/SHA256SUMS" > "$work_dir/verified" || fail 'invalid SHA256SUMS manifest'
(CDPATH='' cd -- "$work_dir" && sha256sum --check --strict verified) || fail 'checksum verification failed'
for tool in $tools; do
	chmod 0755 "$work_dir/$tool-linux-amd64" || fail "cannot make $tool executable"
	reported=$("$work_dir/$tool-linux-amd64" version && printf '.') || fail "version command failed: $tool"
	expected=$(printf '%s\n.' "$tool $version")
	[ "$reported" = "$expected" ] || fail "unexpected version for $tool: ${reported%.}"
done
mkdir -p -- "$bin_dir" || fail "cannot create bin directory: $bin_dir"
check_targets
stage_dir=$(mktemp -d "$bin_dir/.zinc-install.XXXXXX") || fail 'cannot create local staging directory'
for tool in $tools; do
	mode=0755
	[ ! -f "$bin_dir/$tool" ] || { mode=$(stat -c '%a' -- "$bin_dir/$tool") || fail "cannot read permissions: $tool"; }
	cp -- "$work_dir/$tool-linux-amd64" "$stage_dir/$tool" || fail "cannot stage: $tool"
	chmod "$mode" "$stage_dir/$tool" || fail "cannot set permissions: $tool"
done
check_targets
for tool in $tools; do
	mv -fT -- "$stage_dir/$tool" "$bin_dir/$tool" || fail "cannot replace: $tool"
	published="$published $tool"
done
printf 'Installed Zinc %s (zc zcr zlg zlt zvr) in %s\n' "$version" "$bin_dir"
PATH=$bin_dir:${PATH:-}
export PATH

if [ -n "${HOME:-}" ]; then
	quoted=$(printf '%s' "$bin_dir" | sed "s/'/'\\\\''/g") || fail 'cannot quote PATH'
	entry="export PATH='$quoted':\"\$PATH\""
	case "${SHELL##*/}" in
		bash|'') profile=$HOME/.bashrc ;;
		zsh) profile=${ZDOTDIR:-$HOME}/.zshrc ;;
		fish)
			profile=${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/zinc.fish
			quoted=$(printf '%s' "$bin_dir" | sed "s/\\\\/\\\\\\\\/g;s/'/\\\\'/g") || fail 'cannot quote fish PATH'
			entry="fish_add_path -- '$quoted'" ;;
		sh|dash|ksh) profile=$HOME/.profile ;;
		*) fail "automatic PATH setup is unsupported for shell: ${SHELL:-unset}" ;;
	esac
	case "$profile" in /*) ;; *) fail 'shell configuration path must be absolute' ;; esac
	[ ! -L "$profile" ] || fail "shell startup file is a symlink: $profile"
	[ ! -e "$profile" ] || [ -f "$profile" ] || fail "shell startup file is not regular: $profile"
	mkdir -p "$(dirname "$profile")" || fail 'cannot create shell configuration directory'
	append=1
	if [ -f "$profile" ]; then
		[ -r "$profile" ] || fail "cannot read shell startup file: $profile"
		ending=$(tail -c 1 -- "$profile") || fail "cannot read shell startup file: $profile"
		if [ "$ending" = "\\" ]; then
			LC_ALL=C awk 'END { value=$0; count=0; while (sub(/\\$/, "", value)) count++; exit(count%2) }' "$profile" ||
				fail "shell startup file ends in an unescaped backslash: $profile"
		fi
		if grep -Fxq -- "$entry" "$profile"; then append=0
		else result=$?; [ "$result" -eq 1 ] || fail "cannot search shell startup file: $profile"; fi
	fi
	if [ "$append" -eq 1 ]; then
		printf '\n%s\n' "$entry" >> "$profile" || fail "cannot configure PATH in $profile"
	fi
	printf 'PATH configured in %s. Open a new terminal to use Zinc outside this installer.\n' "$profile"
else
	printf '%s\n' 'HOME is unset; PATH is active only inside this installer.'
fi
[ "$example" -eq 1 ] || exit 0

sample=$PWD/zinc-hello.yaml
[ ! -L "$sample" ] || fail "example is a symlink: $sample"
if [ ! -e "$sample" ]; then
	curl -q --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
		--output "$work_dir/example.yaml" \
		"https://raw.githubusercontent.com/crispuscrew/zinc/$version/common/examples/apps/zinc-hello.yaml" || fail 'example download failed'
	printf '%s\n' 'Resolving the example image...'
	pin=$(zc image resolve docker.io/library/alpine:3.22) || fail 'cannot resolve Alpine (check rootless Podman)'
	case "$pin" in docker.io/library/alpine@sha256:*) digest=${pin#*@sha256:} ;; *) fail 'invalid example image pin' ;; esac
	case "$digest" in ''|*[!0-9a-f]*) fail 'invalid example digest' ;; esac
	[ "${#digest}" -eq 64 ] || fail 'invalid example digest length'
	sed "/^  SourceTag:/d;s|^  Image: .*|  Image: $pin\\
  SourceTag: docker.io/library/alpine:3.22|" "$work_dir/example.yaml" > "$work_dir/zinc-hello.yaml" || fail 'cannot prepare YAML'
	zc validate "$work_dir/zinc-hello.yaml" --resolved >/dev/null || fail 'invalid example YAML'
	sample_stage=$(mktemp "$PWD/.zinc-hello.XXXXXX") || fail 'cannot stage example'
	cp "$work_dir/zinc-hello.yaml" "$sample_stage" || fail 'cannot stage example YAML'
	ln "$sample_stage" "$sample" || fail 'example already exists; kept unchanged'
fi
[ -f "$sample" ] || fail "example is not a regular file: $sample"
printf '\n%s\n' "$sample"
cat "$sample" || fail 'cannot display example'
printf '\nRun this example? [y/N] '
IFS= read -r answer || answer=n
case "$answer" in y|Y|yes|YES|Yes) ;; *) printf '%s\n' 'Kept zinc-hello.yaml for you to inspect or edit.'; exit 0 ;; esac

[ -n "${WAYLAND_DISPLAY:-}${DISPLAY:-}" ] || fail 'running the example requires a desktop display'
terminal=${ZINC_TERMINAL:-${TERMINAL:-}}
if [ -z "$terminal" ]; then
	for candidate in foot kitty 'alacritty -e' 'gnome-terminal --' 'konsole -e' 'xterm -e'; do
		case "$candidate" in
			foot) [ -n "${WAYLAND_DISPLAY:-}" ] || continue ;;
			'xterm -e') [ -n "${DISPLAY:-}" ] || continue ;;
		esac
		if command -v "${candidate%% *}" >/dev/null 2>&1; then terminal=$candidate; break; fi
	done
fi
[ -n "$terminal" ] || fail 'no terminal found; set ZINC_TERMINAL and retry'
ZINC_TERMINAL=$terminal
export ZINC_TERMINAL
resolved=$(zc validate "$sample" --resolved) || fail 'invalid example YAML'
image=$(printf '%s\n' "$resolved" | awk '$1 == "Image:" { print $2; exit }')
[ -n "$image" ] || fail 'example has no image'
case "$image" in localhost/*) ;; *)
	podman image exists "$image" || podman pull -- "$image" || fail 'cannot fetch example image' ;;
esac
zc run "$sample" --exec || fail 'example launch failed'
