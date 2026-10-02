#!/bin/sh
# Fails when version pins that must move together have drifted:
#   - Go major.minor: src/go.mod `go` directive vs Dockerfile `ARG GO_VERSION=`
#   - golangci-lint: ci.yml golangci-lint-action `version:` vs Dockerfile COPY tag
#   - GoReleaser: release.yml goreleaser-action `version:` major vs .goreleaser.yaml `version:`
#   - actions/setup-go: `go-version` must equal go.mod major.minor and
#     `go-version-file` must point at a go.mod (ci.yml and release.yml)
#   - version ldflags: the `-X <pkg>.<Var>` flags in the Makefile (VERSION_PKG),
#     Dockerfile and .goreleaser.yaml must name `<go.mod module>/internal/version`
#     and variables that exist as package-level `var`s there (the Go linker
#     silently ignores -X flags naming missing symbols, giving a 'dev' binary)
#   - .golangci.yml `version:` major must equal the pinned golangci-lint major
# These ldflags/golangci.yml mismatches exit 2; other mismatches exit 1.
# `--print-go-version` prints only the go.mod major.minor and exits.
# Paths can be overridden with GO_MOD, DOCKERFILE, CI_YML, RELEASE_YML,
# GORELEASER_YAML, MAKEFILE, VERSION_PKG_DIR and GOLANGCI_YML.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
GO_MOD=${GO_MOD:-$root/src/go.mod}
DOCKERFILE=${DOCKERFILE:-$root/Dockerfile}
CI_YML=${CI_YML:-$root/.github/workflows/ci.yml}
RELEASE_YML=${RELEASE_YML:-$root/.github/workflows/release.yml}
GORELEASER_YAML=${GORELEASER_YAML:-$root/.goreleaser.yaml}
MAKEFILE=${MAKEFILE:-$root/Makefile}
VERSION_PKG_DIR=${VERSION_PKG_DIR:-$root/src/internal/version}
GOLANGCI_YML=${GOLANGCI_YML:-$root/.golangci.yml}

# step_values FILE ACTION KEY: print the value of KEY (in the step's `with:`)
# for every workflow step whose `uses:` contains ACTION. Steps are delimited by
# list items, so key order within a step does not matter.
step_values() {
	awk -v action="$2" -v key="$3" '
		function flush() { if (used != "" && index(used, action) && val != "") print val; used = ""; val = "" }
		/^[[:space:]]*-[[:space:]]/ { flush(); sub(/^[[:space:]]*-[[:space:]]+/, "") }
		{
			line = $0
			sub(/^[[:space:]]+/, "", line)
			if (line ~ /^uses:/) { v = line; sub(/^uses:[[:space:]]*/, "", v); used = v }
			else if (line ~ ("^" key ":") && val == "") {
				v = line; sub("^" key ":[[:space:]]*", "", v)
				sub(/[[:space:]]+#.*$/, "", v); sub(/[[:space:]]+$/, "", v); gsub(/["\047]/, "", v)
				val = v
			}
		}
		END { flush() }
	' "$1"
}

# 1.23.0 -> 1.23
mod_go=$(sed -n 's/^go[[:space:]][[:space:]]*\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' "$GO_MOD" | head -n 1)
if [ -z "$mod_go" ]; then
	echo "error: no 'go' directive found in $GO_MOD" >&2
	exit 2
fi

if [ "${1:-}" = "--print-go-version" ]; then
	echo "$mod_go"
	exit 0
fi

docker_go=$(sed -n 's/^ARG GO_VERSION=\([^[:space:]]*\).*/\1/p' "$DOCKERFILE" | head -n 1)
docker_lint=$(sed -n 's/^COPY --from=golangci\/golangci-lint:\([^[:space:]]*\).*/\1/p' "$DOCKERFILE" | head -n 1)
ci_lint=$(step_values "$CI_YML" 'golangci/golangci-lint-action' version | head -n 1)
gr_workflow=$(step_values "$RELEASE_YML" 'goreleaser/goreleaser-action' version | head -n 1)
gr_config=$(sed -n 's/^version:[[:space:]]*["'\'']*\([0-9][0-9]*\).*/\1/p' "$GORELEASER_YAML" | head -n 1)
# "~> v2", "v2.4.1", "2" -> 2 (empty for "latest" and other unpinned values)
gr_major=$(printf '%s\n' "$gr_workflow" | sed -n 's/^[^0-9]*\([0-9][0-9]*\).*/\1/p')
fail=0
for pair in "go.mod go directive:$mod_go" "Dockerfile ARG GO_VERSION:$docker_go" \
	"Dockerfile golangci-lint tag:$docker_lint" "ci.yml golangci-lint version:$ci_lint" \
	"release.yml goreleaser version:$gr_workflow" ".goreleaser.yaml version:$gr_config"; do
	if [ -z "${pair#*:}" ]; then
		echo "error: could not extract ${pair%%:*}" >&2
		fail=1
	fi
done
[ "$fail" -eq 0 ] || exit 2

if [ "$mod_go" != "$docker_go" ]; then
	echo "MISMATCH Go version: $GO_MOD says $mod_go but $DOCKERFILE has ARG GO_VERSION=$docker_go" >&2
	fail=1
fi
if [ "$ci_lint" != "$docker_lint" ]; then
	echo "MISMATCH golangci-lint: $CI_YML has version $ci_lint but $DOCKERFILE COPY uses $docker_lint" >&2
	fail=1
fi
if [ -z "$gr_major" ] || [ "$gr_major" != "$gr_config" ]; then
	echo "MISMATCH GoReleaser: $RELEASE_YML pins version '$gr_workflow' but $GORELEASER_YAML has version: $gr_config" >&2
	fail=1
fi
for f in "$CI_YML" "$RELEASE_YML"; do
	for v in $(step_values "$f" 'actions/setup-go' go-version); do
		case $v in
		"$mod_go" | "$mod_go".*) ;;
		*) echo "MISMATCH setup-go: $f has go-version $v but $GO_MOD says $mod_go" >&2; fail=1 ;;
		esac
	done
	for v in $(step_values "$f" 'actions/setup-go' go-version-file); do
		case $v in
		*go.mod) ;;
		*) echo "MISMATCH setup-go: $f has go-version-file $v, expected a go.mod" >&2; fail=1 ;;
		esac
	done
done

# --- version ldflags paths and .golangci.yml schema version (exit 2 on drift) ---
lfail=0

# declared_vars DIR: names of package-level vars in DIR/*.go (non-test files).
declared_vars() {
	for f in "$1"/*.go; do
		case $f in *_test.go) continue ;; esac
		[ -f "$f" ] && awk '
			/^var[[:space:]]*\(/ { inblk = 1; next }
			inblk && /^\)/ { inblk = 0; next }
			inblk { l = $0; sub(/^[[:space:]]+/, "", l); if (match(l, /^[A-Za-z_][A-Za-z0-9_]*/)) print substr(l, 1, RLENGTH); next }
			/^var[[:space:]]+[A-Za-z_]/ { l = $0; sub(/^var[[:space:]]+/, "", l); match(l, /^[A-Za-z0-9_]+/); print substr(l, 1, RLENGTH) }
		' "$f"
	done
}

mod_path=$(sed -n 's/^module[[:space:]][[:space:]]*"\{0,1\}\([^"[:space:]]*\).*/\1/p' "$GO_MOD" | head -n 1)
want_pkg=$mod_path/internal/version
if [ -z "$mod_path" ]; then
	echo "error: no 'module' directive found in $GO_MOD" >&2
	exit 2
fi
if [ ! -d "$VERSION_PKG_DIR" ]; then
	echo "error: version package dir $VERSION_PKG_DIR not found" >&2
	exit 2
fi
known_vars=$(declared_vars "$VERSION_PKG_DIR")

# Makefile: VERSION_PKG must itself be the version package; $(VERSION_PKG) in -X is resolved.
mk_pkg=$(sed -n 's/^VERSION_PKG[[:space:]]*:\{0,1\}=[[:space:]]*\([^[:space:]]*\).*/\1/p' "$MAKEFILE" | head -n 1)
if [ "$mk_pkg" != "$want_pkg" ]; then
	echo "MISMATCH ldflags: $MAKEFILE VERSION_PKG is '$mk_pkg' but expected '$want_pkg' (module path from $GO_MOD)" >&2
	lfail=1
fi

for f in "$MAKEFILE" "$DOCKERFILE" "$GORELEASER_YAML"; do
	flags=$(grep -oE '(^|[[:space:]])-X[[:space:]]+[^=[:space:]]+' "$f" | sed 's/^[[:space:]]*-X[[:space:]]*//' || true)
	if [ -z "$flags" ]; then
		echo "error: no '-X <pkg>.<Var>' ldflags found in $f" >&2
		lfail=1
		continue
	fi
	for tok in $flags; do
		case $tok in
		'$(VERSION_PKG)'.*) tok=$mk_pkg.${tok#*.} ;;
		esac
		pkg=${tok%.*}
		var=${tok##*.}
		if [ "$pkg" != "$want_pkg" ]; then
			echo "MISMATCH ldflags: $f has -X $tok but package path should be '$want_pkg' (module path from $GO_MOD)" >&2
			lfail=1
		elif ! printf '%s\n' "$known_vars" | grep -qx "$var"; then
			echo "MISMATCH ldflags: $f has -X $tok but '$var' is not a package-level var in $VERSION_PKG_DIR" >&2
			lfail=1
		fi
	done
done

# .golangci.yml `version:` major vs pinned golangci-lint major (skipped if no file).
if [ -f "$GOLANGCI_YML" ]; then
	gl_cfg=$(sed -n 's/^version:[[:space:]]*["'\'']*\([0-9][0-9]*\).*/\1/p' "$GOLANGCI_YML" | head -n 1)
	gl_pin=$(printf '%s\n' "$ci_lint" | sed -n 's/^[^0-9]*\([0-9][0-9]*\).*/\1/p')
	if [ -z "$gl_cfg" ] || [ "$gl_cfg" != "$gl_pin" ]; then
		echo "MISMATCH golangci-lint config: $GOLANGCI_YML has version '$gl_cfg' but pinned golangci-lint is '$ci_lint'" >&2
		lfail=1
	fi
fi
[ "$lfail" -eq 0 ] || exit 2

[ "$fail" -eq 0 ] || exit 1
echo "ok: Go $mod_go, golangci-lint $ci_lint, GoReleaser v$gr_config"
