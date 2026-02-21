
**Prompt for IPXTransporter Project Specification (Full, Updated – Peer‑Aware Stats)**

---

### Title
**IPXTransporter – High‑Performance, TLS‑Enabled IPX/SPX Traffic Daemon**

### Status: Implementation in Progress
- [x] Dual-mode operation (TUI/Daemon)
- [x] Reliable packet capture (gopacket/pcap)
- [x] TLS Peer Management (TLS 1.3)
- [x] Deduplication logic
- [x] Statistics aggregation & HTTP API
- [x] Terminal UI (tview)
- [x] Interface discovery and selection
- [x] TUI Configuration editor and file browser
- [x] TUI reorganization (stats at bottom, human-readable units)
- [x] Demo mode for UI testing (--demo flag)
- [x] Man page and documentation cleanup
- [x] Network map and Whois lookup overlay
- [x] TUI Traffic graph with colorful dots, full-width scaling, and dynamic time range (+/- zoom)
- [x] Network map with English labels and improved markers
- [x] Realistic demo mode with truly random publicly routable IPv4/IPv6 addresses
- [x] Mouse support in TUI
- [x] Sorting by various fields in TUI and Web UI
- [x] Network topology graph in Web UI (vis.js) with hierarchical connectivity
- [x] Node topology map in TUI with hierarchical connectivity
- [x] Resizable network topology graph in Web UI
- [x] Admin login and peer management (Ban/Disconnect) in Web UI
- [x] Peer management menu in TUI (Disconnect/Ban/Whois) with mouse support
- [x] Ban persistence and enforcement (by ID and Host/IP)
- [x] Demo mode property adjustment in TUI and Web UI
- [x] Dynamic mock peer count adjustment and network chaining in demo mode
- [x] Configurable Admin Password in TUI and Web UI
- [x] Human-readable uptime display (d/h/m/s) in TUI and Web UI
- [x] Double-click to open action menu in TUI
- [x] Max child connection limit (default 5) with consumption display and UI configuration
- [x] Truly random publicly routable IPv4/IPv6 addresses in demo mode
- [x] Examples directory with sample configuration and make install target
- [x] Human-friendly Web UI fields (Connected at/Last seen) and split Children column
- [x] Fixed Web UI demo settings reset issue during user editing
- [x] Proper README.md and documentation cleanup
- [x] Unit tests (TDD) - *Partially implemented*
- [x] Integration tests

### Summary 
IPXTransporter is a Go daemon that captures raw IPX/SPX packets, deduplicates them, and forwards them securely over TLS to peers. Peer traffic is injected back into the local network, and a responsive terminal UI (tview) plus an optional HTTP API delivers statistics in both JSON and a rendered HTML page, including a live list of connected peers.

---

### TDD & Testing Strategy
This project follows Test-Driven Development (TDD) principles. Core logic is covered by unit tests:
1. **Deduplication (`internal/relay/dedup_test.go`)**: Verifies the LRU-based deduplication and eviction.
2. **Configuration (`internal/config/config_test.go`)**: Ensures defaults are applied and JSON loading works correctly.
3. **Statistics (`internal/stats/stats_test.go`)**: Validates data aggregation and model integrity.

*Note: Tests requiring `libpcap` may fail if the library is not installed on the build machine. Use mock-based or isolated tests for core logic.*

---

### Objectives
1. **Dual‑mode operation**
   * **Server mode** – accepts TCP/TLS connections, relays captured packets, and injects peer traffic into the local network.
   * **TUI mode** – displays live stats, traffic graphs, error counts, peer list, and per‑peer traffic counters.

2. **Reliable packet capture** – Prefer `github.com/google/gopacket/pcap`; fall back to raw sockets (`/dev/bpf` on FreeBSD, `AF_PACKET` on Linux).

3. **Intelligent switching** – Forward packets to a known local host when the destination is in a routing table; otherwise broadcast to all peers.

4. **Exactly‑once relay & loop‑prevention** – In‑memory LRU cache (TTL = 30 s) keyed by `(src, dst, txID)`; drop duplicates from peers to avoid loops.

5. **Full‑duplex, per‑peer goroutines** – Two goroutines per peer (`send`, `recv`) plus a central relay loop; all streams use `context.Context`.

6. **TLS enforcement** – Production builds always require certificates from Certbot paths; `--disable-ssl` is debug‑only.

7. **Extensible configuration** – JSON config file (default `/etc/ipxtransporter.json` or `/usr/local/etc/ipxtransporter.json`) overrides any CLI flag.

8. **User‑friendly UI** – `github.com/rivo/tview` + `github.com/gdamore/tcell` for live graphs, peer tables, and traffic counters.

9. **Scalable concurrency** – Support ≥ 100 peers with zero packet loss.

---

### Architecture Overview

```
+----------------+   pkt   +-------------+   pkt   +-----------+
| Capture Layer  |  -->   |  Relay Loop |  -->   |  Peer TLS |
+----------------+         +-------------+         +-----------+
      ^                                       |
      |                                       v
  (gopacket)                          (handshake / gossip)
```  
*All components communicate via channels; shared state (peer list, stats, dedup cache) is protected by `sync.RWMutex` or `sync.Map`.*

---

### High‑Level Requirements

| Category   | Requirement |
|------------|-------------|
| **Performance** | ≥ 5 000 pps, ≤ 200 ms end‑to‑end latency, 100+ peers without loss. |
| **Security** | TLS 1.3, optional mutual cert auth, certificate paths per OS, no compression. |
| **Reliability** | Auto‑reconnect, gossip timeout handling, graceful shutdown via context. |
| **Scalability** | 64 k entry dedup ring, 1 000 sized send/recv channels per peer. |
| **Usability** | TUI refresh every 500 ms, color‑coded traffic levels, JSON API (`/stats`, `/diagram`), live peer table. |
| **Deployment** | `make` builds the binary. `make deb` and `make rpm` produce Debian/RPM packages in `dist/`. A `ports/` sub‑directory is created containing the FreeBSD port files. |
| **License** | 3‑Clause BSD; header in every source file. |

---

### Data Model – Statistics

```go
// Stats holds all metrics that the web API and TUI expose.
type Stats struct {
    TotalReceived  uint64          `json:"total_received"`
    TotalForwarded uint64          `json:"total_forwarded"`
    TotalDropped   uint64          `json:"total_dropped"`
    TotalErrors    uint64          `json:"total_errors"`
    Uptime         time.Duration   `json:"uptime"`
    Peers          []PeerStat      `json:"peers"`          // <‑‑ new: per‑peer metrics
}

// PeerStat captures traffic & health for an individual peer.
type PeerStat struct {
    ID          string          `json:"id"`          // e.g. hostname or TLS cert subject
    IP          net.IP          `json:"ip"`
    ConnectedAt time.Time       `json:"connected_at"`
    LastSeen    time.Time       `json:"last_seen"`
    SentBytes   uint64          `json:"sent_bytes"`
    RecvBytes   uint64          `json:"recv_bytes"`
    SentPkts    uint64          `json:"sent_pkts"`
    RecvPkts    uint64          `json:"recv_pkts"`
    Errors      uint64          `json:"errors"`
}
```

*The `collectStats()` routine must aggregate per‑peer counters into the `Peers` slice and expose the full `Stats` object.*

---

### Web API – Rendered HTML View of Stats (Peer‑Aware)

#### End‑points

| Endpoint          | Method | Content‑Type | Purpose |
|-------------------|--------|--------------|---------|
| `GET /stats`      | GET    | `application/json` | Raw data for scripts, dashboards, or the HTML view’s JavaScript. |
| `GET /stats.html` | GET    | `text/html`        | Renders a human‑friendly page showing stats. |
| `GET /`           | GET    | `text/html`        | Redirects to `/stats.html`. |

> **Dual endpoints** keep the canonical JSON data source while providing an interactive browser view.

#### Rendering Strategy

1. **Go Template** – use `html/template` (or an embedded engine such as `jet`/`pongo2`).
2. **Handler** – serve both JSON and HTML based on the request path suffix.

```go
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
    if strings.HasSuffix(r.URL.Path, ".html") {
        renderStatsPage(w, s.collectStats())
    } else {
        json.NewEncoder(w).Encode(s.collectStats())
    }
}
```

3. **Template (`templates/stats.tmpl`)** – cards for aggregate metrics and a table for peers.

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>IPXTransporter – Stats</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 2rem; }
        .card { border: 1px solid #ddd; padding: 1rem; margin-bottom: 1rem; border-radius: 4px; }
        .card h3 { margin-top: 0; }
        table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
        th, td { border: 1px solid #ddd; padding: 0.5rem; text-align: left; }
        th { background: #f0f0f0; }
    </style>
</head>
<body>
    <h1>IPXTransporter Statistics</h1>
    <div class="card"><h3>Total Packets Received</h3><p>{{ .TotalReceived }}</p></div>
    <div class="card"><h3>Total Packets Forwarded</h3><p>{{ .TotalForwarded }}</p></div>
    <div class="card"><h3>Total Packets Dropped</h3><p>{{ .TotalDropped }}</p></div>
    <div class="card"><h3>Total Errors</h3><p>{{ .TotalErrors }}</p></div>
    <div class="card"><h3>Uptime</h3><p>{{ .Uptime }}</p></div>

    <h2>Connected Peers ({{ len .Peers }})</h2>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>IP</th>
                <th>Connected At</th>
                <th>Last Seen</th>
                <th>Sent (bytes)</th>
                <th>Recv (bytes)</th>
                <th>Sent (pkts)</th>
                <th>Recv (pkts)</th>
                <th>Errors</th>
            </tr>
        </thead>
        <tbody>
        {{ range .Peers }}
            <tr>
                <td>{{ .ID }}</td>
                <td>{{ .IP }}</td>
                <td>{{ .ConnectedAt }}</td>
                <td>{{ .LastSeen }}</td>
                <td>{{ .SentBytes }}</td>
                <td>{{ .RecvBytes }}</td>
                <td>{{ .SentPkts }}</td>
                <td>{{ .RecvPkts }}</td>
                <td>{{ .Errors }}</td>
            </tr>
        {{ else }}
            <tr><td colspan="9">No peers connected.</td></tr>
        {{ end }}
        </tbody>
    </table>
</body>
</html>
```

4. **Optional Client‑Side Refresh** – the `/stats` page can embed a small script that fetches the raw JSON and updates the DOM every few seconds, keeping the page lightweight while remaining interactive.

```html
<script>
    async function loadStats() {
        const resp = await fetch('/stats?raw=true');
        const data = await resp.json();

        // Update aggregate cards
        document.getElementById('total-received').textContent = data.total_received;
        /* … similar updates for other cards … */

        // Update peer table
        const tbody = document.getElementById('peer-table-body');
        tbody.innerHTML = '';
        if (data.peers.length === 0) {
            tbody.innerHTML = '<tr><td colspan="9">No peers connected.</td></tr>';
            return;
        }
        data.peers.forEach(p => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${p.id}</td>
                <td>${p.ip}</td>
                <td>${p.connected_at}</td>
                <td>${p.last_seen}</td>
                <td>${p.sent_bytes}</td>
                <td>${p.recv_bytes}</td>
                <td>${p.sent_pkts}</td>
                <td>${p.recv_pkts}</td>
                <td>${p.errors}</td>
            `;
            tbody.appendChild(tr);
        });
    }
    loadStats();
    setInterval(loadStats, 5000); // refresh every 5 s
</script>
```

---

### Terminal UI – Peer Table

The TUI should present a `tview.Table` that updates every 500 ms with the same `PeerStat` data. The table columns correspond to the fields in `PeerStat`. A highlight or color (e.g., green for active, red for stale) can be applied based on `LastSeen`.

---

### Makefile & Packaging

| Target | Action |
|--------|--------|
| `make` | Displays help message (default). |
| `make build` | Builds the binary (Linux/FreeBSD‑compatible). |
| `make install-deps` | Installs system dependencies (`libpcap`) on Ubuntu/Debian and FreeBSD. |
| `make run` | Runs the app in TUI mode with `--disable-ssl`. |
| `make run-daemon` | Runs the app in daemon mode with `--disable-ssl`. |
| `make run-demo` | Runs the app in TUI mode with demo data. |
| `make demo` | Shortcut for `run-demo`. |
| `make man` | Opens the man page. |
| `make test` | Executes unit tests for core logic. |
| `make fmt` | Formats the source code. |
| `make vet` | Runs `go vet` for static analysis. |
| `make deb` | Creates a Debian package in `dist/`. |
| `make rpm` | Creates an RPM package in `dist/`. |

---

### License
Include a `LICENSE` file with the 3‑Clause BSD text. Add a header comment to every Go file:

```go
// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// <short description of this file>
```

---