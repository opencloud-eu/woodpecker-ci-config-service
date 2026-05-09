SHELL := bash -O globstar

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

DOCKER_IMAGE ?= opencloudeu/wccs
DOCKER_TAG ?= dev
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_OUTPUT ?=

.PHONY: docker-buildx
docker-buildx:
	docker buildx build \
		--platform $(DOCKER_PLATFORMS) \
		--file Dockerfile \
		--tag $(DOCKER_IMAGE):$(DOCKER_TAG) \
		$(DOCKER_OUTPUT) \
		.
