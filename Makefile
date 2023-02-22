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

updatecontainer:
	aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws/a6j2k0g1
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t public.ecr.aws/a6j2k0g1/event-forwarder:latest .