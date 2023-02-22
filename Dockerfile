# syntax=docker/dockerfile:1

## Build
FROM golang:1.18-buster AS build

WORKDIR /usr/local/go/src/github.com/spyderbat/event-forwarder

COPY ./ ./
RUN go mod tidy
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /event-forwarder ./spyderbat-event-forwarder

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

