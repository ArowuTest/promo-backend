# Use Go 1.24 on Alpine as build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Install git so 'go mod download' can fetch modules
RUN apk add --no-cache git

# Copy go.mod and go.sum, download deps
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire repository
COPY . .

# Build the binary
RUN go build -o /promo-backend ./cmd/server

# --------------------------------------
# Final image: scratch or minimal Alpine
# --------------------------------------
FROM alpine:3.18

# Necessary CA certs (for HTTPS everything)
RUN apk add --no-cache ca-certificates

# Copy only the compiled binary from builder
COPY --from=builder /promo-backend /usr/local/bin/promo-backend

# Expose default port (adjust if you override)
ENV PORT=8080
EXPOSE 8080

# When container starts, run the binary
ENTRYPOINT ["/usr/local/bin/promo-backend"]
