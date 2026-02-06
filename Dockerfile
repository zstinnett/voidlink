FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest as builder

WORKDIR /app

# Copy module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /usr/bin/amneziawg-go ./cmd/runner

# Runtime stage
# We use wolfi-base to allow installing packages (iproute2) while keeping it minimal.
FROM cgr.dev/chainguard/wolfi-base:latest

LABEL org.opencontainers.image.source = "$$REPO_URL$$"

# Install iproute2 for 'ip' command support and libcap-utils for setting capabilities
RUN apk add --no-cache iproute2 libcap-utils

# Copy the binary
COPY --from=builder /usr/bin/amneziawg-go /usr/bin/amneziawg-go

# Grant various networking capabilities to the binary so it can run as non-root
# CAP_NET_ADMIN: Create TUN interface, modify IP addresses/routes
# We also set it on the 'ip' binary so the executed commands work
RUN setcap cap_net_admin=+ep /usr/bin/amneziawg-go && \
    setcap cap_net_admin=+ep $(which ip)

# Set working directory to configuration volume
WORKDIR /config

# Run as non-root user (Wolfi/Distroless standard UID 65532)
# Requires running with --cap-add=NET_ADMIN to use 'ip' command
USER 65532:65532

# Basic healthcheck to ensure binary is executable
HEALTHCHECK --interval=30s --timeout=3s \
    CMD ip link show wg0 || exit 1

# Entrypoint
ENTRYPOINT ["/usr/bin/amneziawg-go"]
