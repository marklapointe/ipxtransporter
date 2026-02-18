## Project name
- **IPXTransporter**

## Summary
IPXTransporter is a high‑performance Go daemon that captures IPX/SPX packets, relays them securely over TLS to peers, and forwards peer traffic back to the local network, all while providing a live TUI for statistics and peer management.

## High‑level requirements

1. **Dual‑mode operation**
    - **Server mode** – accepts TCP connections from peers, relays IPX/SPX packets captured on the local interface to all connected peers, and forwards packets received from peers to the local network.
    - **TUI mode** – runs a terminal UI that shows live statistics and colourful text‑based graphs of traffic, packet rates, error counts, and the peer list.

2. **Packet capture**
    - Prefer the `gopacket` library with the `pcap` backend.  If pcap is unavailable, fall back to a platform‑specific raw socket implementation.
    - Support capturing on Linux (`/dev/bpf`/`pcap`) and FreeBSD (`/dev/bpf`).

3. **Packet inspection & switching**
    - Inspect each IPX packet for an identifiable target address.
    - If the target is known locally (present in a pre‑configured routing table or a static list), forward it only to that local host; otherwise broadcast to all peers.  This reduces traffic and increases efficiency.

4. **Relay logic & deduplication**
    - When a packet arrives locally, forward it **once** to every connected peer.
    - When a packet arrives from a peer, forward it to the local network only if it has not been seen locally (prevent infinite loops).
    - Use an in‑memory cache keyed by a hash of source, destination, and a 48‑bit transaction ID.
    - Cache entries expire after **30 seconds** (TTL).  Implement this with a time‑based LRU cache.

5. **Multi‑threaded, full‑duplex traffic streams**
    - For each peer, run **two dedicated goroutines**:
        1. **Send stream** – consumes packets from a channel and writes them over the TLS connection.
        2. **Receive stream** – reads packets from the TLS connection and forwards them to the local network.
    - The server also run a **central relay goroutine** that pushes captured packets into per‑peer send channels.
    - This design ensures that sending and receiving never block each other, maximising throughput.

6. **Peer management**
    - Peers connect via TCP on a configurable port (default 9000).
    - Each peer must send a JSON handshake on connection:  
      `{"type":"handshake","name":"peer‑name","capabilities":["ipx"],"secure":true}`
    - The server keeps a thread‑safe list of peers, displays them in the TUI, and tracks packet counts.

7. **TLS / SSL support (mandatory)**
    - Secure mode is **always enabled** in production.  The server loads TLS certificates from the paths used by Certbot:
        - Linux: `/etc/letsencrypt/live/<domain>/fullchain.pem` and `/etc/letsencrypt/live/<domain>/privkey.pem`
        - FreeBSD: `/usr/local/etc/letsencrypt/live/<domain>/fullchain.pem` and `/usr/local/etc/letsencrypt/live/<domain>/privkey.pem`
    - For debugging, an optional flag `--disable-ssl` may be provided, but the default behaviour must enforce TLS.
    - No compression is used – packet size is minimised by stripping any unused fields from the IPX header before forwarding.  The code should document how the header is packed.

8. **Configuration file**
    - Default location for the JSON config file:
        - Linux: `/etc/ipxtransporter.json`
        - FreeBSD: `/usr/local/etc/ipxtransporter.json`
    - The config file should allow overriding any of the command‑line flags (port, interface, log level, secure mode, peer routing table, etc.).

9. **TUI details (using `github.com/rivo/tview`)**
    - Display the following panes in a single window:
        1. **Statistics panel** – total packets received, forwarded, dropped, errors, uptime.
        2. **Traffic graph** – scrolling line chart of packets/sec (use a simple custom drawing routine with `tcell` colours).
        3. **Peer list** – name, IP, last seen, packet count.
    - Color scheme: green (healthy), yellow (≥100 pkt/s), red (≥200 pkt/s).
    - Update every 500 ms.

10. **Concurrency & performance**
    - Separate goroutines for packet capture, each peer’s send/receive streams, the relay loop, and the TUI update loop.
    - Protect shared state with `sync.RWMutex` or `sync.Map`.
    - Scale to 100+ peers without packet loss.

11. **Command‑line flags**
    - `--mode=[server|tui]`
    - `--port=9000`
    - `--interface=eth0`
    - `--log-level=[debug|info|warn|error]`
    - `--disable-ssl` (only for debugging; default is TLS enforced)
    - `--config=/path/to/file` (default `/etc/ipxtransporter.json` or `/usr/local/etc/ipxtransporter.json`)
    - `--help`

## Packaging & build

1. **Makefile**
    - `make` → build binary for the current OS (`ipxtransporter`).
    - `make linux` → build for Linux (amd64, arm64).
    - `make freebsd` → build for FreeBSD (amd64, arm64).
    - `make deb` → generate a Debian package in `dist/`.
    - `make rpm` → generate an RPM package in `dist/`.
    - `make ports` → generate a FreeBSD port `Makefile` and files in `ports/security/ipxtransporter/`.

2. **Debian packaging**
    - Provide `debian/` directory with `control`, `postinst`, `prerm`, `changelog`, etc.
    - Include the config file template and a systemd service unit.

3. **RPM packaging**
    - Provide a `.spec` file with the build instructions and post‑install scripts.

4. **FreeBSD ports**
    - Provide a `Makefile` and `pkg-plist` that install the binary, config template, and optional systemd (if applicable).

5. **README.md**
    - A brief description (above) that explains the purpose of the project.
    - Build instructions (`make`, `make linux`, `make freebsd`).
    - Run instructions for server/TUI modes.
    - How to add peers.
    - TLS configuration notes.
    - Packaging notes.

## License

- Include a `LICENSE` file containing the **3‑Clause BSD license** text.
- In every source file, place a header comment with the license text and the author information, e.g.: