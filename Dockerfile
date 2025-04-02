# syntax=docker/dockerfile:1

## Deploy debian:stable-slim
FROM debian@sha256:70b337e820bf51d399fa5bfa96a0066fbf22f3aa2c3307e2401b91e2207ac3c3
RUN DEBIAN_FRONTEND="noninteractive" apt-get update && apt-get upgrade -y && apt-get install -y ca-certificates

WORKDIR /opt/local/spyderbat

COPY ./event-forwarder event-forwarder
COPY ./example_config.yaml config.yaml

RUN adduser spyderbat
RUN mkdir -p /opt/local/spyderbat/var/log
RUN chown -R spyderbat /opt/local/spyderbat
USER spyderbat

ENTRYPOINT ["./event-forwarder", "-c", "./config.yaml"]

