FROM golang:1.24.4-alpine AS builder

ARG VERSION

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-s -w -X main.version=${VERSION}" -o /out/vault-client-count-exporter .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/vault-client-count-exporter /vault-client-count-exporter

EXPOSE 9090

ENTRYPOINT ["/vault-client-count-exporter"]
