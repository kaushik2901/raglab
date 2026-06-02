FROM golang:tip-alpine3.23
RUN apk add --no-cache git ca-certificates
WORKDIR /workspace
