# Stage 1: Build binary
FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o substat ./cmd/substat

# Stage 2: Minimal runtime image
FROM alpine:3.19

WORKDIR /app

# ca-certificates for TLS probes, iputils for ping probe, tzdata for correct timezone handling
RUN apk add --no-cache ca-certificates tzdata iputils

COPY --from=builder /build/substat /app/substat
COPY config.example.yaml /app/config.yaml

EXPOSE 8080

VOLUME ["/app/data"]

ENTRYPOINT ["/app/substat"]
CMD ["-config", "/app/config.yaml"]
