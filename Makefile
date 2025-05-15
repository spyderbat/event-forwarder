FLAGS=-trimpath -ldflags "-s -w"
VER=$(shell git describe --tags --long --always)
BINS=spyderbat-event-forwarder.x86_64 spyderbat-event-forwarder.aarch64
FILES=$(BINS) example_config.yaml spyderbat-event-forwarder.service install.sh README.md

.PHONY: help tests release deploy clean builder updatecontainer prune

help:
	@echo please visit README.md

tests:
	go test -cover ./...

spyderbat-event-forwarder.x86_64:
	GOARCH=amd64 go build $(FLAGS) -o spyderbat-event-forwarder.x86_64 ./spyderbat-event-forwarder

spyderbat-event-forwarder.aarch64:
	GOARCH=arm64 go build $(FLAGS) -o spyderbat-event-forwarder.aarch64 ./spyderbat-event-forwarder

release: clean tests spyderbat-event-forwarder.x86_64 spyderbat-event-forwarder.aarch64
	tar cfz spyderbat-event-forwarder.$(VER).tgz $(FILES)
	@echo '>>>' spyderbat-event-forwarder.$(VER).tgz

deploy: release
	sudo ./install.sh
	sudo systemctl restart spyderbat-event-forwarder.service
	sudo journalctl -fu spyderbat-event-forwarder.service

clean:
	rm -f $(BINS) *.tgz container-amd64 container-arm64

builder:
	@if ! docker buildx ls | grep -q mybuilder; then \
		docker buildx create --name mybuilder --use; \
	else \
		docker buildx use mybuilder; \
	fi
	aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws/a6j2k0g1

container-arm64: Dockerfile builder $(BINS)
	cp spyderbat-event-forwarder.aarch64 event-forwarder
	docker buildx build --platform=linux/arm64 --provenance=false --push -t public.ecr.aws/a6j2k0g1/event-forwarder:latest-arm64 .
	rm -f event-forwarder
	touch container-arm64

container-amd64: Dockerfile builder $(BINS)
	cp spyderbat-event-forwarder.x86_64 event-forwarder
	docker buildx build --platform=linux/amd64 --provenance=false --push -t public.ecr.aws/a6j2k0g1/event-forwarder:latest-amd64 .
	rm -f event-forwarder
	touch container-amd64

updatecontainer: container-arm64 container-amd64
	-docker manifest rm public.ecr.aws/a6j2k0g1/event-forwarder:latest
	docker manifest create public.ecr.aws/a6j2k0g1/event-forwarder:latest \
		--amend public.ecr.aws/a6j2k0g1/event-forwarder:latest-amd64 \
		--amend public.ecr.aws/a6j2k0g1/event-forwarder:latest-arm64
	docker manifest push public.ecr.aws/a6j2k0g1/event-forwarder:latest

prune:
	docker buildx prune -f
