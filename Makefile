.PHONY: test install-lint checks build dist

export GOLANGCI_LINT_VERSION=v1.59.1

default: gotip test checks build

gopath:
	mkdir $(CURDIR)/gopath

gopath/bin/gotip: gopath
	GOPATH=$(CURDIR)/gopath go install golang.org/dl/gotip@latest

gotip: gopath/bin/gotip
	$(CURDIR)/gopath/bin/gotip download

test: gotip
	GOOS=wasip1 GOARCH=wasm $(CURDIR)/gopath/bin/gotip test -v -cover ./...

build: 
	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 $(CURDIR)/gopath/bin/gotip build -buildmode=c-shared -trimpath -o plugin.wasm .

install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin ${GOLANGCI_LINT_VERSION}

checks:
	GOOS=wasip1 GOARCH=wasm golangci-lint run

dist: build
	cp plugin.wasm dist/traefik-oidc-wasm
	cp .traefik.yml dist/traefik-oidc-wasm