# syntax=docker/dockerfile:1

## Build (golang v1.22)
FROM golang@sha256:ef61a20960397f4d44b0e729298bf02327ca94f1519239ddc6d91689615b1367 AS build


# This cannot be in /usr/local/go or the vcs embedding breaks
WORKDIR /tmp/event-forwarder

COPY ./ ./
RUN go build -trimpath -ldflags "-s -w" -o /event-forwarder ./spyderbat-event-forwarder

## Deploy debian:stable-slim
FROM debian@sha256:4255c9f8a4d6e66488adc0c2084c99df44bda22849b21b3afc0e9746e9a0be18

WORKDIR /opt/local/spyderbat

COPY --from=build /event-forwarder event-forwarder
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY ./example_config.yaml config.yaml

RUN adduser spyderbat
RUN mkdir -p /opt/local/spyderbat/var/log
RUN chown -R spyderbat /opt/local/spyderbat
USER spyderbat

ENTRYPOINT ["./event-forwarder", "-c", "./config.yaml"]

