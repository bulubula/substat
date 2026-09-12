# substat

> **Subpath-Native, Zero-Config, Single-Binary Uptime Monitor**

A lightweight, modern status page and network probe designed specifically for homelabs, reverse proxies, and stealth subpath deployments.

## Highlights

- 🌐 **Subpath-Native**: Effortlessly host under any sub-path (e.g. `/zymstat/`) behind Nginx, Caddy, or Traefik without path rewriting hacks.
- ⚡ **Lightweight & Single Binary**: Written in Go with embedded static assets (`go:embed`). Minimal memory footprint (<15MB).
- 🔍 **Versatile Probing**: ICMP, TCP handshake, TLS latency, DNS resolution, and HTTP/HTTPS status assertion.
- 🔒 **Zero Exposure**: Designed to blend seamlessly into hardened, non-root web architectures.
