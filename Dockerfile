ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/workerd     ./cmd/workerd     && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/api         ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git openssh-client
COPY --from=builder /out/* /usr/local/bin/

WORKDIR /workspace
CMD ["workerd"]
