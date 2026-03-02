APP=qdash
IMAGE ?= ghcr.io/arencloud/qdash:dev
CONTAINER_TOOL ?= podman
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= -s -w -X github.com/arencloud/qdash/internal/version.Version=$(VERSION) -X github.com/arencloud/qdash/internal/version.Commit=$(COMMIT) -X github.com/arencloud/qdash/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: run build test tidy smoke-post swagger-gen openshift-dev-up image-build image-push

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/server

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

smoke-post:
	./scripts/post_deploy_smoke.sh

swagger-gen:
	./scripts/swagger_gen.sh

openshift-dev-up:
	./scripts/openshift_dev_up.sh

image-build:
	$(CONTAINER_TOOL) build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE) .

image-push:
	$(CONTAINER_TOOL) push $(IMAGE)
