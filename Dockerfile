# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
ARG TARGETARCH

WORKDIR /opencloud-eu/woodpecker-config-service

COPY go.mod go.sum ./
RUN go mod download

COPY testenv/conf/clean ./testenv/conf/clean
COPY main.go ./
RUN GOOS=linux GOARCH="${TARGETARCH}" go build -o bin/woodpecker-config-service .

FROM alpine:3.21

LABEL maintainer="OpenCloud GmbH <devops@opencloud.eu>" \
        org.opencontainers.image.title="OpenCloud" \
        org.opencontainers.image.vendor="OpenCloud GmbH" \
        org.opencontainers.image.authors="OpenCloud GmbH" \
        org.opencontainers.image.description="OpenCloud advanced woodpecker configuration service" \
        org.opencontainers.image.documentation="https://github.com/opencloud-eu/woodpecker-ci-config-service" \
        org.opencontainers.image.source="https://github.com/opencloud-eu/woodpecker-ci-config-service"

COPY --from=build /opencloud-eu/woodpecker-config-service/bin/woodpecker-config-service /usr/bin/woodpecker-config-service

EXPOSE 8080/tcp
ENTRYPOINT ["woodpecker-config-service"]
