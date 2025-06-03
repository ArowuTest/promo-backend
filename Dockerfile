# syntax=docker/dockerfile:1.4

#####################################
# 1) Builder Stage: Compile the Go binary
#####################################
FROM golang:1.24-alpine AS builder

# 1.1) Install Git (needed if your modules come from git)
#     Install ca-certificates in case any go-fetch uses HTTPS (not strictly needed here)
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# 1.2) Copy only go.mod/go.sum first, download deps (caches layer effectively)
COPY go.mod go.sum ./
RUN go mod tidy

# 1.3) Copy the entire source tree
COPY . .

# 1.4) Build a fully static binary (disable CGO, target Linux amd64)
#     Output binary at /promo-backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /promo-backend ./cmd/server

#####################################
# 2) Runtime Stage: Minimal Alpine
#####################################
FROM alpine:3.18

# 2.1) Install only CA certificates (for any HTTPS calls your Go code makes)
RUN apk add --no-cache ca-certificates

# 2.2) Copy the compiled binary from the builder stage
COPY --from=builder /promo-backend /usr/local/bin/promo-backend

# 2.3) Expose the port your Gin server listens on
#     You can override PORT at runtime via environment variables if you wish
ENV PORT=8080
EXPOSE 8080

# 2.4) Run the binary when the container starts
ENTRYPOINT ["/usr/local/bin/promo-backend"]
