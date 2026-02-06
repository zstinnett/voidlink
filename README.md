# Distroless AmneziaWG-Go on Wolfi

This project provides a secure, minimal, and distroless container image for running [AmneziaWG](https://github.com/amnezia-vpn/amneziawg-go) (a WireGuard fork with obfuscation). It is built on top of [Chainguard Wolfi](https://edu.chainguard.dev/open-source/wolfi/overview/) and uses a custom Go runner to handle configuration natively.

## Features

- **Distroless**: Built on Wolfi, containing only the binary and minimal dependencies (`iproute2`).
- **Secure**: Minimized attack surface.
- **Rootless Capable**: Can run in rootless Podman/Docker (requires `CAP_NET_ADMIN` and `/dev/net/tun`).
- **Config Compatible**: Supports standard `wg-quick` INI syntax plus Amnezia extensions.

## Usage

### Prerequisites

- Host with `tun` module loaded.
- Docker or Podman.

### Quick Start

1.  **Prepare Config**: Create a `wg0.conf` file.
    ```ini
    [Interface]
    PrivateKey = <YOUR_PRIVATE_KEY>
    Address = 10.0.0.1/24
    ListenPort = 51820
    Jc = 10
    Jmin = 50
    Jmax = 1000
    S1 = 15
    S2 = 25
    H1 = 1
    H2 = 2

    [Peer]
    PublicKey = <PEER_PUBLIC_KEY>
    AllowedIPs = 10.0.0.2/32
    ```

2.  **Run with Docker**:
    ```bash
    docker run -d \
      --name amneziawg \
      --cap-add=NET_ADMIN \
      --device=/dev/net/tun \
      -v $(pwd)/wg0.conf:/config/wg0.conf:ro \
      -p 51820:51820/udp \
      $$IMAGE_NAME$$
    ```

3.  **Run with Podman (Rootless)**:
    ```bash
    podman run -d \
      --name amneziawg \
      --cap-add=NET_ADMIN \
      --device=/dev/net/tun \
      -v $(pwd)/wg0.conf:/config/wg0.conf:ro \
      -p 51820:51820/udp \
      $$IMAGE_NAME$$
    ```

## Configuration Parameters

### Standard
- `PrivateKey`, `ListenPort`, `FwMark`, `Address`, `MTU`.
- `[Peer]`: `PublicKey`, `PresharedKey`, `Endpoint`, `PersistentKeepalive`, `AllowedIPs`.

### Amnezia Specific
- `Jc`: Junk packet count.
- `Jmin`: Junk packet minimum size.
- `Jmax`: Junk packet maximum size.
- `H1`, `H2`, `H3`, `H4`: Header types.

## Security Best Practices

> [!IMPORTANT]
> This container implements multiple security hardening measures. Follow these guidelines for secure deployment.

### 1. Configuration File Security

**File Permissions**: The runner enforces strict permissions on the configuration file:
```bash
# Required: 0600 or more restrictive
chmod 600 wg0.conf
```

**Never Commit Secrets**: Do NOT commit configuration files with real private keys to version control.
- Use `.gitignore` to exclude `*.conf` files
- Use secrets management for production deployments

### 2. Secrets Management

**Docker Swarm**:
```yaml
services:
  amneziawg:
    image: $$IMAGE_NAME$$
    secrets:
      - wg0_config
    command: ["/config/wg0.conf"]
    volumes:
      - /run/secrets/wg0_config:/config/wg0.conf:ro

secrets:
  wg0_config:
    external: true
```

**Kubernetes**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: amneziawg-config
data:
  wg0.conf: <base64-encoded-config>
---
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: amneziawg
    volumeMounts:
    - name: config
      mountPath: /config
      readOnly: true
  volumes:
  - name: config
    secret:
      secretName: amneziawg-config
      defaultMode: 0600
```

### 3. Input Validation

The runner automatically validates:
- ✅ IP addresses (CIDR format) to prevent command injection
- ✅ Amnezia parameters (`Jmin < Jmax`)
- ✅ Config file permissions (0600 required)

### 4. Network Isolation

**Production Deployment**:
- Use dedicated network namespaces
- Limit capabilities to only `NET_ADMIN`
- Never run with `--privileged`

**Example with restricted capabilities**:
```bash
docker run -d \
  --cap-add=NET_ADMIN \
  --cap-drop=ALL \
  --device=/dev/net/tun \
  --read-only \
  --security-opt=no-new-privileges \
  -v $(pwd)/wg0.conf:/config/wg0.conf:ro \
  $$IMAGE_NAME$$
```

### Build Locally
```bash
go build ./cmd/runner
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Repository

- **Source**: [$$REPO_URL$$]($$REPO_URL$$)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
