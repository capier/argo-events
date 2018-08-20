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
DOCKER_PUSH=true
IMAGE_NAMESPACE=metalgearsolid
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
all: sensor-linux sensor-controller-linux gateway-controller-linux gateway-transformer-linux

docker-all: sensor-image sensor-controller-image gateway-controller-image gateway-transformer-image webhook-image

docker-gateway-example: webhook-image

.PHONY: all sensor-controller sensor-controller-image gateway-controller gateway-controller-image clean test

# Sensor
sensor:
	go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/sensor ./sensor-controller/cmd/

sensor-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make sensor

sensor-image: sensor-linux
	docker build -t $(IMAGE_PREFIX)sensor:$(IMAGE_TAG) -f ./sensor-controller/cmd/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)sensor:$(IMAGE_TAG) ; fi

# Sensor controller
sensor-controller:
	go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/sensor-controller ./cmd/sensor-controller

sensor-controller-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make sensor-controller

sensor-controller-image: sensor-controller-linux
	docker build -t $(IMAGE_PREFIX)sensor-controller:$(IMAGE_TAG) -f ./sensor-controller/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)sensor-controller:$(IMAGE_TAG) ; fi

# Gateway controller
gateway-controller:
	go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/gateway-controller ./cmd/gateway-controller/main.go

gateway-controller-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make gateway-controller

gateway-controller-image: gateway-controller-linux
	docker build -t $(IMAGE_PREFIX)gateway-controller:$(IMAGE_TAG) -f ./gateway-controller/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)gateway-controller:$(IMAGE_TAG) ; fi

# Gateway transformer
gateway-transformer:
	go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/gateway-transformer ./cmd/gateway-controller/transform/main.go

gateway-transformer-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make gateway-transformer

gateway-transformer-image: gateway-transformer-linux
	docker build -t $(IMAGE_PREFIX)gateway-transformer:$(IMAGE_TAG) -f ./gateway-controller/transform/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)gateway-transformer:$(IMAGE_TAG) ; fi


# gateway binaries
webhook:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags '${LDFLAGS}' -o ${DIST_DIR}/webhook-gateway ./signals/webhook/

webhook-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make webhook

webhook-image: webhook-linux
	docker build -t $(IMAGE_PREFIX)webhook-gateway:$(IMAGE_TAG) -f ./signals/webhook/Dockerfile .
	@if [ "$(DOCKER_PUSH)" = "true" ] ; then docker push $(IMAGE_PREFIX)webhook-gateway:$(IMAGE_TAG) ; fi

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