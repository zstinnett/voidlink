FROM cgr.dev/chainguard/go:latest as builder

WORKDIR /app

# Copy module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 go build -o /usr/bin/amneziawg-go ./cmd/runner

# Runtime stage
# We use wolfi-base to allow installing packages (iproute2) while keeping it minimal.
FROM cgr.dev/chainguard/wolfi-base:latest

# Install iproute2 for 'ip' command support
RUN apk add --no-cache iproute2

# Copy the binary
COPY --from=builder /usr/bin/amneziawg-go /usr/bin/amneziawg-go

# Set working directory to configuration volume
WORKDIR /config

# Run as non-root user (Wolfi/Distroless standard UID 65532)
# Requires running with --cap-add=NET_ADMIN to use 'ip' command
USER 65532:65532

# Basic healthcheck to ensure binary is executable
HEALTHCHECK --interval=30s --timeout=3s \
    CMD /usr/bin/amneziawg-go --help || exit 1

# Entrypoint
ENTRYPOINT ["/usr/bin/amneziawg-go"]
