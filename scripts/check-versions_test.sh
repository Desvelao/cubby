#!/bin/sh
# Fixture-based tests for check-versions.sh. Fixtures are generated in a temp
# dir and passed through the script's *_YML / GO_MOD / DOCKERFILE overrides.
set -u

here=$(cd "$(dirname "$0")" && pwd)
script=$here/check-versions.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

passed=0
failed=0

fixtures() {
	cat >"$tmp/go.mod" <<'EOT'
module example.com/x

go 1.23.0
EOT
	cat >"$tmp/Dockerfile" <<'EOT'
ARG GO_VERSION=1.23
FROM golang:${GO_VERSION}-bookworm AS build
RUN go build -ldflags="-s -w -X example.com/x/internal/version.Version=${VERSION} -X example.com/x/internal/version.Commit=${COMMIT}" ./...
FROM golang:${GO_VERSION}-bookworm AS dev
COPY --from=golangci/golangci-lint:v2.1 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
EOT
	cat >"$tmp/ci.yml" <<'EOT'
jobs:
  test:
    steps:
      - uses: actions/setup-go@v5
        with:
          go-version-file: src/go.mod
      - name: Lint
        uses: golangci/golangci-lint-action@v7
        with:
          version: v2.1
          working-directory: src
EOT
	cat >"$tmp/release.yml" <<'EOT'
jobs:
  goreleaser:
    steps:
      - uses: actions/setup-go@v5
        with:
          go-version-file: src/go.mod
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
EOT
	printf 'version: 2\nproject_name: x\nbuilds:\n  - ldflags:\n      - -s -w\n      - -X example.com/x/internal/version.Version={{.Version}}\n      - -X example.com/x/internal/version.Date={{.Date}}\n' >"$tmp/goreleaser.yaml"
	printf 'VERSION_PKG := example.com/x/internal/version\nLDFLAGS := -s -X $(VERSION_PKG).Version=$(V) -X $(VERSION_PKG).Commit=$(C)\n' >"$tmp/Makefile"
	mkdir -p "$tmp/version"
	printf 'package version\n\nvar (\n\tVersion = "dev"\n\tCommit  = "none"\n)\n\nvar Date = "unknown"\n' >"$tmp/version/version.go"
	printf 'version: "2"\nlinters:\n  default: standard\n' >"$tmp/golangci.yml"
}

# run NAME WANT_RC [PATTERN]: run the script on the current fixtures.
run() {
	name=$1 want=$2 pat=${3:-}
	out=$(GO_MOD=$tmp/go.mod DOCKERFILE=$tmp/Dockerfile CI_YML=$tmp/ci.yml \
		RELEASE_YML=$tmp/release.yml GORELEASER_YAML=$tmp/goreleaser.yaml MAKEFILE=$tmp/Makefile \
		VERSION_PKG_DIR=$tmp/version GOLANGCI_YML=$tmp/golangci.yml sh "$script" 2>&1)
	rc=$?
	if [ "$rc" -ne "$want" ]; then
		echo "FAIL $name: exit $rc, want $want"; echo "$out"; failed=$((failed + 1)); return
	fi
	if [ -n "$pat" ] && ! printf '%s\n' "$out" | grep -q "$pat"; then
		echo "FAIL $name: output lacks '$pat'"; echo "$out"; failed=$((failed + 1)); return
	fi
	echo "ok   $name"
	passed=$((passed + 1))
}

fixtures
run "matching pins pass" 0 "^ok:"

fixtures; sed -i 's/^go 1.23.0/go 1.24.0/' "$tmp/go.mod"
run "Go drift fails" 1 "MISMATCH Go version"

fixtures; sed -i 's/golangci-lint:v2.1/golangci-lint:v2.2/' "$tmp/Dockerfile"
run "golangci-lint drift fails" 1 "MISMATCH golangci-lint"

fixtures; sed -i 's/version: v2.1/version: v2.3/' "$tmp/ci.yml"
run "golangci-lint drift in ci.yml fails" 1 "MISMATCH golangci-lint"

fixtures
cat >"$tmp/ci.yml" <<'EOT'
jobs:
  test:
    steps:
      - with:
          version: v2.1
          working-directory: src
        uses: golangci/golangci-lint-action@v7
        name: Lint
      - uses: actions/setup-go@v5
        with:
          go-version-file: src/go.mod
EOT
run "reordered keys (version above uses) pass" 0 "^ok:"

fixtures
cat >"$tmp/ci.yml" <<'EOT'
jobs:
  test:
    steps:
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache: true
      - with:
          version: v2.9
        uses: golangci/golangci-lint-action@v7
EOT
run "reordered keys drift still detected" 1 "MISMATCH golangci-lint"

fixtures; sed -i 's/version: "~> v2"/version: "~> v1"/' "$tmp/release.yml"
run "GoReleaser major drift fails" 1 "MISMATCH GoReleaser"

fixtures; sed -i 's/version: "~> v2"/version: latest/' "$tmp/release.yml"
run "GoReleaser unpinned fails" 1 "MISMATCH GoReleaser"

fixtures; sed -i 's/version: "~> v2"/version: v2.4.1 # pinned/' "$tmp/release.yml"
run "GoReleaser exact v2 passes" 0 "^ok:"

fixtures; sed -i 's/^version: 2/version: 3/' "$tmp/goreleaser.yaml"
run ".goreleaser.yaml drift fails" 1 "MISMATCH GoReleaser"

fixtures; sed -i 's/go-version-file: src\/go.mod/go-version: "1.22"/' "$tmp/ci.yml"
run "setup-go go-version drift fails" 1 "MISMATCH setup-go"

fixtures; sed -i 's/go-version-file: src\/go.mod/go-version: "1.23"/' "$tmp/release.yml"
run "setup-go go-version match passes" 0 "^ok:"

fixtures; sed -i 's/go-version-file: src\/go.mod/go-version-file: src\/go.sum/' "$tmp/release.yml"
run "setup-go go-version-file not go.mod fails" 1 "MISMATCH setup-go"

fixtures; : >"$tmp/release.yml"
run "missing goreleaser pin is an error" 2 "could not extract"

fixtures; sed -i 's#example.com/x/internal/version#example.com/x/internal/versoin#' "$tmp/Makefile"
run "drifted VERSION_PKG in Makefile fails" 2 "MISMATCH ldflags: .*Makefile"

fixtures; sed -i 's#example.com/x/internal/version#example.com/y/internal/version#g' "$tmp/Dockerfile"
run "drifted ldflags package in Dockerfile fails" 2 "MISMATCH ldflags: .*Dockerfile"

fixtures; sed -i 's#example.com/x/internal/version#example.com/x/version#' "$tmp/goreleaser.yaml"
run "drifted ldflags package in goreleaser fails" 2 "MISMATCH ldflags: .*goreleaser.yaml"

fixtures; sed -i 's#version.Commit=#version.Commmit=#' "$tmp/Dockerfile"
run "unknown ldflags variable fails" 2 "not a package-level var"

fixtures; rm "$tmp/golangci.yml"
run "missing .golangci.yml is skipped" 0 "^ok:"

fixtures; sed -i 's/^version: "2"/version: "1"/' "$tmp/golangci.yml"
run ".golangci.yml major drift fails" 2 "MISMATCH golangci-lint config"

fixtures
out=$(GO_MOD=$tmp/go.mod sh "$script" --print-go-version 2>&1)
if [ "$out" = "1.23" ]; then echo "ok   --print-go-version"; passed=$((passed + 1)); else echo "FAIL --print-go-version: $out"; failed=$((failed + 1)); fi

echo "$passed passed, $failed failed"
[ "$failed" -eq 0 ]
