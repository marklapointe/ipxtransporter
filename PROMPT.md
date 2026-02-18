
**Prompt for IPXTransporter Project Specification (Full, Updated – Peer‑Aware Stats)**

---

### Title
**IPXTransporter – High‑Performance, TLS‑Enabled IPX/SPX Traffic Daemon**

### Summary (≤ 350 characters)
IPXTransporter is a Go daemon that captures raw IPX/SPX packets, deduplicates them, and forwards them securely over TLS to peers. Peer traffic is injected back into the local network, and a responsive terminal UI (tview) plus an optional HTTP API delivers statistics in both JSON and a rendered HTML page, including a live list of connected peers.

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
| `GET /stats/html` | GET    | `text/html` | Renders a human‑friendly page showing the same statistics, **including a table of connected peers**. |

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
| `make` | Builds the binary (Linux/FreeBSD‑compatible). |
| `make deb` | Creates a Debian package in `dist/`. |
| `make rpm` | Creates an RPM package in `dist/`. |
| `make ports` | Generates a minimal FreeBSD port tree inside a `ports/` directory (no separate FreeBSD build target). |

---

### License
Include a `LICENSE` file with the 3‑Clause BSD text. Add a header comment to every Go file:

```go
// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: <your‑name>
// <short description of this file>
```

---