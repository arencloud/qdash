# syntax=docker/dockerfile:1

FROM golang:1.25 AS builder
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags="-s -w -X github.com/egevorky/qdash/internal/version.Version=${VERSION} -X github.com/egevorky/qdash/internal/version.Commit=${COMMIT} -X github.com/egevorky/qdash/internal/version.BuildDate=${BUILD_DATE}" \
  -o /out/qdash ./cmd/server

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /app

COPY --from=builder /out/qdash /app/qdash
COPY web /app/web

RUN chmod 0555 /app/qdash

EXPOSE 8080
USER 1001
ENTRYPOINT ["/app/qdash"]
