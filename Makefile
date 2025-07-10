SHELL := bash -O globstar
DIST_DIR ?= dist

TARGETOS ?= $(shell go env GOOS)
TARGETARCH ?= $(shell go env GOARCH)

HAS_GO = $(shell hash go > /dev/null 2>&1 && echo "GO" || echo "NOGO" )
ifeq ($(HAS_GO),GO)
	XGO_VERSION ?= go-1.24.x
	CGO_CFLAGS ?= $(shell go env CGO_CFLAGS)
endif
CGO_CFLAGS ?=

.PHONY: lint
lint:
	go tool golangci-lint run

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor

format:
	go tool gofumpt -extra -w .

.PHONY: clean
clean:
	go clean -i ./...
	rm -rf bin

add-license:
	go tool addlicense -c 'OpenCloud GmbH' -ignore 'vendor/**' **/*.go

.PHONY: test
test:
	go test -race -cover -coverprofile coverage.out -timeout 60s .

cross-compile:
	$(foreach platform,$(subst ;, ,$(PLATFORMS)),\
		TARGETOS=$(firstword $(subst |, ,$(platform))) \
		TARGETARCH_XGO=$(subst arm64/v8,arm64,$(subst arm/v7,arm-7,$(word 2,$(subst |, ,$(platform))))) \
		TARGETARCH_BUILDX=$(subst arm64/v8,arm64,$(subst arm/v7,arm,$(word 2,$(subst |, ,$(platform))))) \
		make release-server-xgo || exit 1; \
	)
	tree ${DIST_DIR}

release-server-xgo: check-xgo
	@echo "Building for:"
	@echo "os:$(TARGETOS)"
	@echo "arch orgi:$(TARGETARCH)"
	@echo "arch (xgo):$(TARGETARCH_XGO)"
	@echo "arch (buildx):$(TARGETARCH_BUILDX)"

	CGO_CFLAGS="$(CGO_CFLAGS)" xgo -go $(XGO_VERSION) -dest ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX) -ldflags '-linkmode external $(LDFLAGS)' -targets '$(TARGETOS)/$(TARGETARCH_XGO)' -out wccs -pkg cmd/wccs .

	@if [ "$${XGO_IN_XGO:-0}" -eq "1" ]; then \
	  echo "inside xgo image"; \
	  mkdir -p ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX); \
	  mv -vf /build/wccs* ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX)/wccs; \
	else \
	  echo "outside xgo image"; \
	  [ -f "${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX)/wccs" ] && rm -v ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX)/wccs; \
	  mv -v ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_XGO)/wccs* ${DIST_DIR}/$(TARGETOS)_$(TARGETARCH_BUILDX)/wccs; \
	fi

check-xgo:
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install src.techknowlogick.com/xgo@latest; \
	fi
