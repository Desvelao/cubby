# syntax=docker/dockerfile:1

# Go version for the build and dev stages. The default must match the `go`
# directive (major.minor) in src/go.mod; `make check-versions` (run in CI)
# fails on drift. CI and release pass it explicitly via --build-arg.
ARG GO_VERSION=1.23

# The build stage runs on the builder's native platform and cross-compiles for
# the target (TARGETOS/TARGETARCH are set by BuildKit), so multi-arch images
# need no emulation during compilation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src/ .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X github.com/Desvelao/cubby/internal/version.Version=${VERSION} -X github.com/Desvelao/cubby/internal/version.Commit=${COMMIT} -X github.com/Desvelao/cubby/internal/version.Date=${DATE}" \
    -o /out/cubby ./cmd/cubby

# dev is the contributor sandbox: full Go toolchain + linter, source
# bind-mounted at runtime via docker-compose (see docker-compose.yml).
FROM golang:${GO_VERSION}-bookworm AS dev
WORKDIR /workspace/src
# World-writable cache root for HOME and the Go/lint caches so the container can
# run as any host uid (compose mounts a named volume here, inheriting this mode).
RUN mkdir -p /cache && chmod 1777 /cache
# The golangci-lint version here and `version:` in .github/workflows/ci.yml must
# move together (checked by scripts/check-versions.sh).
COPY --from=golangci/golangci-lint:v2.1 /usr/bin/golangci-lint /usr/local/bin/golangci-lint
CMD ["bash"]

# runtime is the minimal published image: just the binary. The :nonroot tag runs
# as uid/gid 65532 by default; override with `docker run --user` when writing to
# a bind mount (see README).
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
# VERSION is re-declared because ARGs do not carry across stages; pass the same
# --build-arg VERSION=... used for the build stage.
ARG VERSION=dev
LABEL org.opencontainers.image.title="cubby" \
      org.opencontainers.image.description="Designs slotted divider-panel inserts for boardgame boxes from a YAML manifest" \
      org.opencontainers.image.source="https://github.com/Desvelao/cubby" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=build /out/cubby /usr/local/bin/cubby
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/cubby"]
