SHELL=/usr/bin/env bash

all: build
.PHONY: all

unexport GOFLAGS

GOCC?=go

BUILD_DEPS:=

meta: $(BUILD_DEPS)
	rm -f meta
	$(GOCC) build $(GOFLAGS) -o meta ./cmd

.PHONY: meta
BINS+=meta

build: meta
	@[[ $$(type -P "meta") ]] && echo "Caution: you have \
	an existing meta binary in your PATH. you can execute make install to /usr/local/bin" || true

.PHONY: build

clean:
	rm -rf $(BINS)
.PHONY: clean

install: install-meta

install-meta:
	install -C ./meta /usr/local/bin
