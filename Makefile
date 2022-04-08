AMD64GCC=x86_64-linux-gnu-gcc-9
ARM64GCC=aarch64-linux-gnu-gcc
FLAGS=-trimpath -ldflags "-s -w"
VER=$(shell git describe --tags --long --always)
BINS=spyderbat-event-forwarder.x86_64 spyderbat-event-forwarder.aarch64
FILES=$(BINS) example_config.yaml spyderbat-event-forwarder.service install.sh README.md

help:
	@echo please visit README.md

release: clean
	CGO_ENABLED=1 CC=$(AMD64GCC) GOARCH=amd64 go build $(FLAGS) -o spyderbat-event-forwarder.x86_64 ./spyderbat-event-forwarder
	CGO_ENABLED=1 CC=$(ARM64GCC) GOARCH=arm64 go build $(FLAGS) -o spyderbat-event-forwarder.aarch64 ./spyderbat-event-forwarder
	tar cfz spyderbat-event-forwarder.$(VER).tgz $(FILES)
	@echo '>>>' spyderbat-event-forwarder.$(VER).tgz

clean:
	rm -f $(BINS) *.tgz
