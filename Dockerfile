# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/oasrelay \
    ./cmd/oasrelay

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/oasrelay /oasrelay

USER 65532:65532
WORKDIR /work

ENTRYPOINT ["/oasrelay"]
