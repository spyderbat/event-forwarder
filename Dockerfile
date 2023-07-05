# syntax=docker/dockerfile:1

## Build
FROM golang:1.20 AS build

# This cannot be in /usr/local/go or the vcs embedding breaks
WORKDIR /tmp/event-forwarder

COPY ./ ./
RUN go build -trimpath -ldflags "-s -w" -o /event-forwarder ./spyderbat-event-forwarder

## Deploy
FROM debian:stable-slim

WORKDIR /opt/local/spyderbat

COPY --from=build /event-forwarder event-forwarder
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY ./example_config.yaml config.yaml

RUN adduser spyderbat
RUN mkdir -p /opt/local/spyderbat/var/log
RUN chown -R spyderbat /opt/local/spyderbat
USER spyderbat

ENTRYPOINT ["./event-forwarder", "-c", "./config.yaml"]

