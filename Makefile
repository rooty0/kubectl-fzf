all: build test

LD_FLAGS=-ldflags "-X 'main.gitCommit=$(shell git rev-parse --short HEAD)' -X 'main.gitBranch=$(shell git rev-parse --abbrev-ref HEAD)' -X 'main.goVersion=$(shell go version)' -X 'main.buildDate=$(shell date -Iseconds -u)' -X 'main.version=$(shell git describe --tags)'"
BINS := kubectl-fzf-completion kubectl-fzf-server

build:
	go build $(LD_FLAGS) ./cmd/kubectl-fzf-server
	go build $(LD_FLAGS) ./cmd/kubectl-fzf-completion

install:
	go install $(LD_FLAGS) ./cmd/kubectl-fzf-server
	go install $(LD_FLAGS) ./cmd/kubectl-fzf-completion

DOCKER_BUILD_ARGS=--build-arg GIT_COMMIT="$(shell git rev-parse --short HEAD)" \
				  --build-arg GIT_BRANCH="$(shell git rev-parse --abbrev-ref HEAD)" \
				  --build-arg VERSION="$(shell git describe --tags)" \
				  --build-arg BUILD_DATE="$(shell date -Iseconds -u)" \
				  --build-arg GO_VERSION="$(shell go version)"

DOCKER_TAGS=-t rooty0/kubectl-fzf:latest \
	-t rooty0/kubectl-fzf:$(shell git describe --tags)

docker:
	docker build . \
		$(DOCKER_TAGS) \
		$(DOCKER_BUILD_ARGS)

docker-minikube:
	eval $$(minikube docker-env) && docker build . \
		$(DOCKER_TAGS) \
		$(DOCKER_BUILD_ARGS)

test:
	go test ./...

# Configured in .golangci.yml. Installs the pinned version into ./bin on
# demand, so the local binary and ci.yml cannot drift apart.
lint: golangci-lint
	"$(GOLANGCI_LINT)" run

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
# Pinned to match .github/workflows/ci.yml; upgrade both together.
GOLANGCI_LINT_VERSION ?= v2.12.2

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): | $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
# Differs from the kubebuilder/kueue-helper original in one portability fix:
# plain `readlink` instead of the GNU-only `readlink --`, which BSD readlink
# (macOS) rejects, making every invocation reinstall the tool.
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

# Kept out of `test` so a machine without zsh can still run the Go suite.
test-shell:
	zsh shell/kubectl_fzf_test.zsh

# All shell suites (zsh, fish, bash, and the fzf end-to-end) in a pristine
# container: nothing but docker and go needed locally.
test-shell-docker:
	bash shell/tests/run_in_docker.sh

snapshot:
	GO_VERSION="$(shell go version)" goreleaser release --snapshot --clean

release:
	GO_VERSION="$(shell go version)" goreleaser release --clean

graph:
	goda graph ./... | dot -Tsvg -o graph.svg

clean:
	go clean
	rm -f $(BINS)
