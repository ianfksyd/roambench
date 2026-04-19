.PHONY: build build-pam build-agent run clean hash-password release-packages

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
LDFLAGS ?= -X main.version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o roambench ./cmd/roambench

build-agent:
	go build -trimpath -ldflags "$(LDFLAGS)" -o roambench-agent ./cmd/roambench-agent

build-all: build build-agent

build-pam:
	go build -tags pam -trimpath -ldflags "$(LDFLAGS)" -o roambench ./cmd/roambench

release-packages:
	scripts/package-roambench-release.sh $(if $(TAG),--tag $(TAG),)

run: build
	./roambench --port 3000

clean:
	rm -f roambench roambench-agent

hash-password: build
	@echo "Enter password (then press Ctrl+D):"
	@./roambench --password-hash
