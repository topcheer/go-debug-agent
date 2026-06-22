FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the demo binary
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/demo ./demo

# ── Runtime stage ──
FROM alpine:3.18

RUN apk add --no-cache sqlite-libs ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/demo /app/demo

EXPOSE 8080

CMD ["/app/demo"]
