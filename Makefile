SHELL=/usr/bin/env bash

all: build
.PHONY: all

unexport GOFLAGS

GOCC?=go

BUILD_DEPS:=

generate-car: $(BUILD_DEPS)
	rm -f generate-car
	$(GOCC) build $(GOFLAGS) -o generate-car ./cmd

.PHONY: generate-car
BINS+=generate-car

build: generate-car
	@[[ $$(type -P "generate-car") ]] && echo "Caution: you have \
	an existing generate-car binary in your PATH. you can execute make install to /usr/local/bin" || true

.PHONY: build

clean:
	rm -rf $(BINS)
.PHONY: clean

install: install-generate-car

install-generate-car:
	install -C ./generate-car /usr/local/bin