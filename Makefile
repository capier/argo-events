PACKAGE=github.com/argoproj/argo-events
CURRENT_DIR=$(shell pwd)
DIST_DIR=${CURRENT_DIR}/dist

VERSION=$(shell cat ${CURRENT_DIR}/VERSION)
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_TAG=$(shell if [ -z "`git status --porcelain`" ]; then git describe --exact-match --tags HEAD 2>/dev/null; fi)
GIT_TREE_STATE=$(shell if [ -z "`git status --porcelain`" ]; then echo "clean" ; else echo "dirty"; fi)

override LDFLAGS += \
  -X ${PACKAGE}.version=${VERSION} \
  -X ${PACKAGE}.buildDate=${BUILD_DATE} \
  -X ${PACKAGE}.gitCommit=${GIT_COMMIT} \
  -X ${PACKAGE}.gitTreeState=${GIT_TREE_STATE}

# docker image publishing options
DOCKER_PUSH=false
IMAGE_NAMESPACE=argoproj
IMAGE_TAG=latest

ifeq (${DOCKER_PUSH},true)
ifndef IMAGE_NAMESPACE
$(error IMAGE_NAMESPACE must be set to push images (e.g. IMAGE_NAMESPACE=argoproj))
endif
endif

ifneq (${GIT_TAG},)
IMAGE_TAG=${GIT_TAG}
override LDFLAGS += -X ${PACKAGE}.gitTag=${GIT_TAG}
endif
ifdef IMAGE_NAMESPACE
IMAGE_PREFIX=${IMAGE_NAMESPACE}/
endif

# Build the project images
.DELETE_ON_ERROR:
all: sensor-controller gateway-controller

.PHONY: all sensor-controller sensor-controller-image gateway-controller gateway-controller-image clean test

# Sensor controller
sensor-controller:
	go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/sensor-controller ./cmd/sensor-controller

sensor-controller-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make sensor-controller

sensor-controller-image: controller-linux
	docker build -t $(IMAGE_PREFIX)sensor-controller:$(IMAGE_TAG) -f ./controller/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)sensor-controller:$(IMAGE_TAG) ; fi

# Gateway controller
gateway-controller:
    go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/gateway-controller ./cmd/gateway-controller/main.go

# Gateway transformer
gateway-transformer:
    go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/gateway-transformer ./cmd/gateway-controller/transform/

# gateway binaries
webhook:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/webhook-signal ./signals/webhook/micro

webhook-image: webhook
	docker build -t $(IMAGE_PREFIX)webhook-signal:$(IMAGE_TAG) -f ./signals/webhook/micro/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)webhook-signal:$(IMAGE_TAG) ; fi

test:
	go test $(shell go list ./... | grep -v /vendor/) -race -short -v

coverage:
	go test -covermode=count -coverprofile=coverage.out $(shell go list ./... | grep -v /vendor/)
	go tool cover -func=coverage.out

clean:
	-rm -rf ${CURRENT_DIR}/dist

.PHONY: protogen
protogen:
	./hack/generate-proto.sh

.PHONY: clientgen
clientgen:
	./hack/update-codegen.sh

.PHONY: openapigen
openapi-gen:
	./hack/update-openapigen.sh

.PHONY: codegen
codegen: clientgen openapigen protogen