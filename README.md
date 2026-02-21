# IPXTransporter

IPXTransporter is a high-performance Go daemon designed to bridge IPX/SPX traffic across modern networks. It captures raw IPX packets from a local interface, deduplicates them to prevent loops, and securely relays them to remote peers over TLS 1.3 encrypted tunnels.

## Key Features

- **Dual-Mode Operation**: Run as a background daemon or with an interactive Terminal UI.
- **Secure Relaying**: Full-duplex communication with peers using TLS 1.3.
- **Hierarchical Topology**: Supports structured relay networks with child connection limits and consumption tracking.
- **Intelligent Deduplication**: Uses a 64k entry LRU cache to ensure "exactly-once" packet delivery.
- **Interactive TUI**:
    - Live traffic graphs with dynamic zoom (`+/-`).
    - Hierarchical network topology map.
    - Peer management (Disconnect/Ban/WHOIS) with mouse support.
    - Configuration editor and file browser.
- **Web Dashboard**:
    - Real-time statistics and interactive topology graph (vis.js).
    - Admin login for remote peer management.
    - Responsive layout with resizable components.
- **Realistic Demo Mode**: High-fidelity simulation with random publicly routable IPs and hierarchical connectivity for testing without a live network.
- **Cross-Platform**: Supports Linux (Ubuntu/Debian) and FreeBSD.

## Installation

### Dependencies

Building IPXTransporter requires `libpcap`.

**Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install libpcap-dev build-essential
```

**FreeBSD:**
```bash
sudo pkg install libpcap
```

Alternatively, use the provided Makefile:
```bash
make install-deps
```

### Building

```bash
make build
```

## Usage

```bash
./ipxtransporter [OPTIONS]
```

### Options

- `--config path`: Path to the JSON configuration file (default: `/etc/ipxtransporter.json`).
- `--interface name`: Network interface to capture from (e.g., `eth0`).
- `--listen addr`: TLS listen address (default: `:8787`).
- `--tui`: Enable Terminal UI mode (default: `true`).
- `--demo`: Enable demo mode with fake traffic for UI testing.
- `--disable-ssl`: Disable TLS (debug only).

### TUI Shortcuts

- `F1`: Configuration Editor
- `F2`: Interface Selection
- `F3`: Peer WHOIS Details
- `F4`: UI Settings (Sorting)
- `F5`: Demo Mode Settings (Demo mode only)
- `F6`: Manual Peer Addition
- `Enter`: Peer Action Menu
- `+/-`: Traffic Graph Zoom
- `Ctrl+C`: Graceful Exit

## Configuration

A sample configuration file (`/etc/ipxtransporter.json`):

```json
{
  "interface": "eth0",
  "listen_addr": ":8787",
  "peers": ["1.2.3.4:8787"],
  "tls_cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
  "tls_key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
  "enable_http": true,
  "http_listen_addr": ":8080",
  "admin_user": "admin",
  "admin_pass": "admin",
  "max_children": 5,
  "network_key": "secret-key",
  "rebalance_enabled": true,
  "rebalance_interval": 30,
  "jwt_secret": "secret-jwt-key"
}
```

## Development

The included `Makefile` provides several targets for development:

- `make build`: Compiles the binary.
- `make run-demo`: Starts the app in demo mode.
- `make test`: Runs unit tests.
- `make fmt`: Formats the code.
- `make vet`: Runs static analysis.
- `make man`: Opens the man page.

## License

This project is licensed under the **3-Clause BSD License**. See the `LICENSE` file for details.

## Author

IPXTransporter is developed and maintained by **Mark LaPointe <mark@cloudbsd.org>**.
