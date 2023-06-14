
# go parameters
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_TEST=$(GO_CMD) test -v
GO_PACKAGE=github.com/winrouter/csi-hostpath

# build info
NAME=csi-hostpath
OUTPUT_DIR=./bin
IMAGE_NAME_FOR_ACR=harbor.archeros.cn/dev/ake/${NAME}
IMAGE_NAME_FOR_DOCKERHUB=harbor.archeros.cn/dev/ake/${NAME}
MAIN_FILE=./cmd/csi-plugin/main.go
LD_FLAGS=-ldflags "-X '${GO_PACKAGE}/pkg/version.GitCommit=$(GIT_COMMIT)' -X '${GO_PACKAGE}/pkg/version.Version=$(VERSION)' -X 'main.VERSION=$(VERSION)' -X 'main.COMMITID=$(GIT_COMMIT)'"
GIT_COMMIT=$(shell git rev-parse HEAD)
VERSION=v0.7.1

CRD_OPTIONS ?= "crd:trivialVersions=true"
CRD_VERSION=v1alpha1

# build binary
all: test fmt vet build

.PHONY: test
test:
	$(GO_TEST) -coverprofile=covprofile ./...
	$(GO_CMD) tool cover -html=covprofile -o coverage.html

.PHONY: build
build:
	CGO_ENABLED=0 $(GO_BUILD) $(LD_FLAGS) -mod=vendor -v -o $(OUTPUT_DIR)/$(NAME) $(MAIN_FILE)

.PHONY: develop
develop:
	GOARCH=amd64 GOOS=linux CGO_ENABLED=0 $(GO_BUILD) $(LD_FLAGS) -v -o $(OUTPUT_DIR)/$(NAME) $(MAIN_FILE)
	chmod +x $(OUTPUT_DIR)/$(NAME)
	docker build . -t ${IMAGE_NAME_FOR_DOCKERHUB}:${VERSION} -f ./Dockerfile.dev

# build image
.PHONY: image
image:
	docker build . -t ${IMAGE_NAME_FOR_DOCKERHUB}:${VERSION} -f ./Dockerfile

# build image for arm64
.PHONY: image-arm64
image-arm64:
	docker build . -t ${IMAGE_NAME_FOR_DOCKERHUB}:${VERSION}-arm64 -f ./Dockerfile.arm64
	docker tag ${IMAGE_NAME_FOR_DOCKERHUB}:${VERSION}-arm64 ${IMAGE_NAME_FOR_ACR}:${VERSION}-arm64

.PHONY: image-tools
image-tools:
	docker build . -t ${IMAGE_NAME_FOR_DOCKERHUB}:tools -f ./Dockerfile.tools
	docker tag ${IMAGE_NAME_FOR_DOCKERHUB}:tools ${IMAGE_NAME_FOR_ACR}:tools

# generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	./hack/update-codegen.sh
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role crd paths="./pkg/apis/storage/$(CRD_VERSION)/..." output:crd:artifacts:config=helm/crds/

.PHONY: fmt
fmt:
	go fmt ./...
.PHONY: vet
vet:
	go vet `go list ./... | grep -v /vendor/`

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0
CONTROLLER_GEN=$(shell go env GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif


grpc-update:
	go get google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
	export PATH="$PATH:$(go env GOPATH)/bin"
	cd pkg/lib
	protoc --go_out=. --go_opt=paths=source_relative   --go-grpc_out=. --go-grpc_opt=paths=source_relative vol.proto