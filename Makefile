.PHONY: build test lint vet fmt examples demos demos-check run docker-dev docker-test docker-build check-versions clean

SRC_DIR := src
BIN := $(SRC_DIR)/bin/cubby
# Build metadata injected like goreleaser and the Dockerfile do. Override on
# the command line, e.g. `make build VERSION=v1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --verify -q HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/Desvelao/cubby/internal/version
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).Date=$(DATE)
EXAMPLES_DIR ?= examples
# Examples whose `build` is expected to exit non-zero, as file=code (space
# separated). Any example not listed must exit 0.
EXAMPLES_EXPECT ?= overfull.yaml=3
# Every exporter is exercised for each example (step is experimental). Each
# format's output file (<box>.<suffix>) must be present and non-empty.
EXAMPLES_FORMATS ?= csv,svg,dxf,stl,step,iso-svg,iso-svg-exploded,iso-png,iso-png-exploded,assembly
EXAMPLES_SUFFIXES ?= csv svg dxf stl step iso-svg.svg iso-svg-exploded.svg iso-png.png iso-png-exploded.png assembly.pdf
# Committed demo artifacts in $(EXAMPLES_DIR), as manifest=formats (formats
# comma separated). Outputs are named after each manifest's box.name, e.g.
# chess-checkers.yaml -> chess-checkers-combo.iso-png.png.
DEMOS ?= chess-checkers.yaml=iso-png,iso-png-exploded,iso-svg \
	plain-joints.yaml=iso-png,iso-png-exploded,assembly \
	height-reduction.yaml=assembly \
	row-dividers.yaml=assembly

build:
	cd $(SRC_DIR) && go build -buildvcs=false -ldflags '$(LDFLAGS)' -o bin/cubby ./cmd/cubby

test:
	cd $(SRC_DIR) && go test ./... -race -cover

vet:
	cd $(SRC_DIR) && go vet ./...

fmt:
	@out=$$(cd $(SRC_DIR) && gofmt -l .); \
	if [ -n "$$out" ]; then echo "unformatted files (run gofmt -w):"; echo "$$out"; exit 1; fi

lint:
	cd $(SRC_DIR) && golangci-lint run ./...

examples: build
	@tmp=$$(mktemp -d) || exit 1; \
	trap 'rm -rf "$$tmp"' EXIT; \
	fail=0; \
	for f in $(EXAMPLES_DIR)/*.yaml; do \
		name=$$(basename "$$f"); \
		want=0; \
		for e in $(EXAMPLES_EXPECT); do \
			if [ "$${e%%=*}" = "$$name" ]; then want=$${e#*=}; fi; \
		done; \
		err=$$($(BIN) validate "$$f" 2>&1 >/dev/null); rc=$$?; \
		if [ $$rc -ne 0 ]; then \
			echo "FAIL $$name: validate exited $$rc (want 0)"; echo "$$err"; fail=1; continue; \
		fi; \
		out="$$tmp/$${name%.yaml}"; mkdir -p "$$out"; \
		err=$$($(BIN) build "$$f" --format $(EXAMPLES_FORMATS) --out-dir "$$out" 2>&1 >/dev/null); rc=$$?; \
		if [ $$rc -ne $$want ]; then \
			echo "FAIL $$name: build exited $$rc (want $$want)"; echo "$$err"; fail=1; continue; \
		fi; \
		if [ $$want -ne 0 ]; then \
			echo "ok   $$name (validate 0, build $$rc, outputs not checked)"; continue; \
		fi; \
		bad=0; \
		set -- "$$out"/*.csv; base=$${1%.csv}; \
		for x in $(EXAMPLES_SUFFIXES); do \
			if [ ! -s "$$base.$$x" ]; then echo "FAIL $$name: missing or empty output $$(basename "$$base").$$x"; bad=1; fi; \
		done; \
		if [ $$bad -ne 0 ]; then fail=1; continue; fi; \
		echo "ok   $$name (validate 0, build $$rc, all exporters)"; \
	done; \
	exit $$fail

# Demo artifacts are compared byte-for-byte by `demos-check`, except files with
# a suffix listed here. Measured by building every demo twice: PNG and SVG
# output is byte-identical between runs, but the assembly PDFs differ every run
# (embedded timestamp/ID), so for those only existence is checked.
DEMOS_NONDETERMINISTIC ?= .pdf

# Shell fragment: render every DEMOS entry into the directory in $$gen. Sets
# fail=1 (and prints FAIL) when a build fails.
define demos_generate
for d in $(DEMOS); do \
	name=$${d%%=*}; fmts=$${d#*=}; \
	if $(BIN) build "$(EXAMPLES_DIR)/$$name" --format "$$fmts" --out-dir "$$gen" >/dev/null; then \
		echo "ok   $$name ($$fmts)"; \
	else \
		echo "FAIL $$name: build failed"; fail=1; \
	fi; \
done
endef

# Renders into a temp dir first and copies into $(EXAMPLES_DIR) only when every
# build succeeded and the destination is writable, so a failed run leaves it
# untouched. Each file is copied to a temp name then renamed over the target.
demos: build
	@gen=$$(mktemp -d) || exit 1; \
	trap 'rm -rf "$$gen"' EXIT; \
	fail=0; \
	$(demos_generate); \
	if [ $$fail -ne 0 ]; then echo "demos failed; $(EXAMPLES_DIR) left untouched"; exit 1; fi; \
	if [ ! -d "$(EXAMPLES_DIR)" ] || [ ! -w "$(EXAMPLES_DIR)" ]; then \
		echo "$(EXAMPLES_DIR) is not a writable directory; nothing copied"; exit 1; \
	fi; \
	for f in "$$gen"/*; do \
		dst="$(EXAMPLES_DIR)/$$(basename "$$f")"; \
		if [ -e "$$dst" ] && [ ! -f "$$dst" ]; then echo "$$dst is not a regular file; nothing copied"; exit 1; fi; \
	done; \
	for f in "$$gen"/*; do \
		dst="$(EXAMPLES_DIR)/$$(basename "$$f")"; \
		if ! { cp "$$f" "$$dst.tmp.$$$$" && mv -f "$$dst.tmp.$$$$" "$$dst"; }; then \
			rm -f "$$dst.tmp.$$$$"; echo "FAIL could not update $$dst"; exit 1; \
		fi; \
		echo "wrote $$dst"; \
	done

# Regenerates every demo into a temp dir and compares with the committed
# artifacts in $(EXAMPLES_DIR) without modifying it. Fails naming stale or
# missing files; see DEMOS_NONDETERMINISTIC for what is compared by existence.
demos-check: build
	@gen=$$(mktemp -d) || exit 1; \
	trap 'rm -rf "$$gen"' EXIT; \
	fail=0; \
	$(demos_generate); \
	if [ $$fail -ne 0 ]; then echo "demos-check: could not regenerate demos"; exit 1; fi; \
	stale=""; \
	for f in "$$gen"/*; do \
		b=$$(basename "$$f"); dst="$(EXAMPLES_DIR)/$$b"; \
		if [ ! -f "$$dst" ]; then stale="$$stale$$b (missing)\n"; continue; fi; \
		skip=0; \
		for s in $(DEMOS_NONDETERMINISTIC); do \
			case "$$b" in *"$$s") skip=1;; esac; \
		done; \
		if [ $$skip -eq 1 ]; then echo "skip $$b (non-deterministic format, existence only)"; continue; fi; \
		if cmp -s "$$f" "$$dst"; then echo "same $$b"; else stale="$$stale$$b (differs)\n"; fi; \
	done; \
	if [ -n "$$stale" ]; then \
		echo "stale demo artifacts in $(EXAMPLES_DIR):"; printf "%b" "$$stale" | sed 's/^/  /'; \
		echo "run make demos"; exit 1; \
	fi; \
	echo "demos up to date"

run: build
	$(BIN) $(ARGS)

# Run as the host user so the bind mount does not get root-owned files.
DOCKER_USER_ENV = LOCAL_UID=$$(id -u) LOCAL_GID=$$(id -g)

docker-dev:
	$(DOCKER_USER_ENV) docker compose run --rm dev

docker-test:
	$(DOCKER_USER_ENV) docker compose run --rm test

docker-build:
	docker build --target runtime -t cubby .

check-versions:
	@sh scripts/check-versions_test.sh
	@sh scripts/check-versions.sh

clean:
	rm -rf $(SRC_DIR)/bin
