FROM golang:1.25.7 AS builder

ARG VERSION

WORKDIR /

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -v -o /vault-usage-exporter -ldflags="-s -w -X main.version=${VERSION}"

EXPOSE 9090
WORKDIR /

ENTRYPOINT ["/vault-usage-exporter"]