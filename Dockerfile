# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
ARG TARGETOS
ARG TARGETARCH

WORKDIR /opencloud-eu/woodpecker-config-service

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -o bin/wccs ./cmd/wccs

FROM scratch

LABEL maintainer="OpenCloud GmbH <devops@opencloud.eu>" \
  org.opencontainers.image.title="OpenCloud woodpecker ci config service" \
  org.opencontainers.image.vendor="OpenCloud GmbH" \
  org.opencontainers.image.authors="OpenCloud GmbH" \
  org.opencontainers.image.description="OpenCloud woodpecker ci config service" \
  org.opencontainers.image.documentation="https://github.com/opencloud-eu/woodpecker-ci-config-service" \
  org.opencontainers.image.source="https://github.com/opencloud-eu/woodpecker-ci-config-service"

COPY --from=build /opencloud-eu/woodpecker-config-service/bin/wccs /usr/bin/wccs

EXPOSE 8080/tcp
ENTRYPOINT ["wccs"]
