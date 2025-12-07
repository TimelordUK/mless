VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build install clean release patch minor

# Default build
build:
	go build $(LDFLAGS) -o mless ./cmd/mless

# Install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/mless

# Clean build artifacts
clean:
	rm -f mless

# Show current version
version:
	@echo $(VERSION)

# Create a new patch release (v0.1.0 -> v0.1.1)
patch:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	patch=$$(echo $$latest | sed 's/v//' | cut -d. -f3); \
	new_patch=$$((patch + 1)); \
	new_version="v$$major.$$minor.$$new_patch"; \
	echo "Bumping $$latest -> $$new_version"; \
	git tag -a $$new_version -m "Release $$new_version"

# Create a new minor release (v0.1.0 -> v0.2.0)
minor:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	major=$$(echo $$latest | sed 's/v//' | cut -d. -f1); \
	minor=$$(echo $$latest | sed 's/v//' | cut -d. -f2); \
	new_minor=$$((minor + 1)); \
	new_version="v$$major.$$new_minor.0"; \
	echo "Bumping $$latest -> $$new_version"; \
	git tag -a $$new_version -m "Release $$new_version"

# Build a release binary
release: clean
	go build $(LDFLAGS) -o mless ./cmd/mless
	@echo "Built mless $(VERSION)"
