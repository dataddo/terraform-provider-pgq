NAME=pgq
BINARY=terraform-provider-${NAME}
VERSION?=0.1.0
OS_ARCH?=darwin_arm64

GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

BUILD_DIR=./bin
LDFLAGS=-ldflags "-X main.version=${VERSION}"

TERRAFORM_PLUGIN_DIR=~/.terraform.d/plugins/registry.terraform.io/dataddo/${NAME}/${VERSION}/${OS_ARCH}

.PHONY: all build install test clean fmt vet tidy help

all: build

build:
	@echo "Building $(BINARY) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY)"

install: build
	@echo "Installing provider to $(TERRAFORM_PLUGIN_DIR)..."
	@mkdir -p $(TERRAFORM_PLUGIN_DIR)
	@cp $(BUILD_DIR)/$(BINARY) $(TERRAFORM_PLUGIN_DIR)/
	@echo "Provider installed successfully!"
	@echo ""
	@echo "To use this provider, add the following to your Terraform configuration:"
	@echo ""
	@echo "terraform {"
	@echo "  required_providers {"
	@echo "    $(NAME) = {"
	@echo "      source  = \"dataddo/$(NAME)\""
	@echo "      version = \"$(VERSION)\""
	@echo "    }"
	@echo "  }"
	@echo "}"

test:
	@echo "Running tests..."
	$(GOTEST) -v -cover ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Make sure PostgreSQL is running and PGHOST, PGDATABASE, PGUSER, PGPASSWORD are set"
	$(GOTEST) -v -tags=integration ./...

clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

tidy:
	@echo "Tidying go.mod..."
	$(GOMOD) tidy

lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

dev: fmt vet build install

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

docs:
	@echo "Generating documentation..."
	@echo "Note: This requires tfplugindocs to be installed"
	@which tfplugindocs > /dev/null || (echo "tfplugindocs not found. Install with: go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest" && exit 1)
	tfplugindocs generate

release:
	@echo "Building release binaries..."
	@mkdir -p $(BUILD_DIR)/release
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY)_$(VERSION)_darwin_amd64
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY)_$(VERSION)_darwin_arm64
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY)_$(VERSION)_linux_amd64
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY)_$(VERSION)_linux_arm64
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY)_$(VERSION)_windows_amd64.exe
	@echo "Release binaries built in $(BUILD_DIR)/release/"

help:
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := build
