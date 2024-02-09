# syntax=docker/dockerfile:1

## Deploy debian:stable-slim
FROM debian@sha256:4255c9f8a4d6e66488adc0c2084c99df44bda22849b21b3afc0e9746e9a0be18
RUN DEBIAN_FRONTEND="noninteractive" apt-get update && apt-get upgrade -y && apt-get install -y ca-certificates

WORKDIR /opt/local/spyderbat

COPY ./event-forwarder event-forwarder
COPY ./example_config.yaml config.yaml

RUN adduser spyderbat
RUN mkdir -p /opt/local/spyderbat/var/log
RUN chown -R spyderbat /opt/local/spyderbat
USER spyderbat

ENTRYPOINT ["./event-forwarder", "-c", "./config.yaml"]

