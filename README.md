# Mikronec

[![Release](https://img.shields.io/github/v/release/aprakasa/mikronec?include_prereleases)](https://github.com/aprakasa/mikronec/releases)
[![Docker](https://img.shields.io/badge/ghcr.io-aprakasa%2Fmikronec-blue)](https://github.com/aprakasa/mikronec/pkgs/container/mikronec)
[![Go Reference](https://pkg.go.dev/badge/github.com/aprakasa/mikronec.svg)](https://pkg.go.dev/github.com/aprakasa/mikronec)
[![Go Report Card](https://goreportcard.com/badge/github.com/aprakasa/mikronec)](https://goreportcard.com/report/github.com/aprakasa/mikronec)
[![License](https://img.shields.io/github/license/aprakasa/mikronec)](LICENSE)

Mikronec (Mikrotik Connector) adalah server backend Go berperforma tinggi yang berfungsi sebagai bridge (jembatan) untuk mengelola dan memantau beberapa router MikroTik melalui satu API yang aman.

Aplikasi ini menggunakan connection multiplexing untuk menggunakan kembali koneksi yang ada dan menyediakan endpoint Server-Sent Events (SSE) untuk pemantauan data real-time (seperti status hardware dan pengguna aktif).

**Swagger UI**: `http://localhost:8080/swagger/index.html`

## Fitur

- **Manajemen Multi-Router**: Kelola banyak router (router_id) dari satu server.
- **Connection Multiplexing**: Sesi koneksi (berdasarkan host|user|pass) dibagikan antar router untuk menghemat resource.
- **Real-time SSE**: Endpoint /sse/{routerID} men-streaming data langsung (CPU, Hotspot Active, PPP Active) ke frontend Anda.
- **Poller Tangguh**: Poller data otomatis dimulai, berhenti (saat tidak ada klien), dan mencoba menyambung ulang jika koneksi terputus.
- **Aman**: Semua endpoint dilindungi oleh middleware API Key statis.

## Konfigurasi

Aplikasi ini dikonfigurasi menggunakan environment variable.

- API_KEY (Wajib): Kunci rahasia (API Key) yang harus - dikirim oleh klien di header X-API-Key untuk otentikasi. Server akan gagal startup jika variabel ini tidak diatur.

- PORT: Port tempat server akan berjalan. Standarnya adalah 8080 (sesuai untuk Google Cloud Run).

## Cara Menjalankan

1. Lokal (Development)
   Pastikan Anda memiliki Go (versi 1.21+) terinstal.

```bash
# Atur variabel lingkungan dan jalankan server
export MY_API_KEY="kunci-rahasia-anda-yang-sangat-aman"
export PORT="8080"

go run .
# Output: ✅ MikroHot Connector (Secure) running at :8080
```

2. Docker (Production)
   Lihat `Dockerfile` di bawah. Anda dapat membangun dan menjalankannya dengan perintah berikut:

```bash
# 1. Bangun image Docker
docker build -t mikronec .

# 2. Jalankan container
docker run -d -p 8080:8080 \
  -e MY_API_KEY="kunci-rahasia-anda-yang-sangat-aman" \
  -e PORT="8080" \
  --name connector \
  mikronec
```

## Penggunaan API

Semua permintaan API wajib menyertakan header `X-API-Key` dan `ALLOWED_ORIGINS`.

```bash
X-API-Key: kunci-rahasia-anda-yang-sangat-aman
ALLOWED_ORIGINS: http://localhost:8080
```

```bash
POST /connect
```

Mendaftarkan `router_id` ke kredensial router tertentu dan memulai koneksi.

Body (JSON):

```bash
{
  "router_id": "router-01",
  "host": "192.168.88.1:8728",
  "user": "admin",
  "pass": "password123"
}
```

Contoh (cURL)

```bash
curl -X POST 'http://localhost:8080/connect' \
-H 'X-API-Key: kunci-rahasia-anda-yang-sangat-aman' \
-H 'Content-Type: application/json' \
-d '{
  "router_id": "router-01",
  "host": "192.168.88.1",
  "user": "admin",
  "pass": "password123"
}'
```

---

```
POST /disconnect
```

Menutup koneksi untuk router_id, menghentikan poller, dan menutup semua klien SSE terkait.

Body (JSON):

```bash
{
  "router_id": "router-01"
}
```

```bash
curl -X POST 'http://localhost:8080/disconnect' \
-H 'X-API-Key: kunci-rahasia-anda-yang-sangat-aman' \
-H 'Content-Type: application/json' \
-d '{"router_id": "router-01"}'
```

---

```
GET /system-info
```

Mengambil informasi sistem dasar dari router yang terkait dengan `router_id`.

Query Parameter: router (string, wajib): `router_id` yang ingin Anda cek.

Contoh (cURL)

```bash
curl 'http://localhost:8080/system-info?router=cabang-jakarta-01' \
-H 'X-API-Key: kunci-rahasia-anda-yang-sangat-aman'
```

---

```bash
POST /run
```

Menjalankan perintah arbitrer di router. Ini adalah endpoint yang sangat powerful dan berbahaya jika API Key Anda bocor.

Body (JSON):

- `router_id` (string): Target router.
- `args` (array of string): Perintah dan argumen yang akan dijalankan.

Contoh (cURL):

```bash
# Contoh untuk mengambil daftar IP address
curl -X POST 'http://localhost:8080/run' \
-H 'X-API-Key: kunci-rahasia-anda-yang-sangat-aman' \
-H 'Content-Type: application/json' \
-d '{
  "router_id": "cabang-jakarta-01",
  "args": ["/ip/address/print"]
}'
```

---

```bash
GET /sse/{routerID}
```

Membuka koneksi Server-Sent Events (SSE) untuk streaming data real-time.

Path Parameter: routerID (string, wajib): `router_id` yang ingin Anda pantau.

Contoh (cURL)

```bash
# Opsi -N (no-buffer) diperlukan untuk melihat stream
curl -N 'http://localhost:8080/sse/cabang-jakarta-01' \
-H 'X-API-Key: kunci-rahasia-anda-yang-sangat-aman'
```

Contoh Output Stream:

```bash
data: {"router_id":"cabang-jakarta-01","hardware":[...],"hotspot_active":[],"ppp_active":[...],"ts":1678886400}

data: {"router_id":"cabang-jakarta-01","hardware":[...],"hotspot_active":[],"ppp_active":[...],"ts":1678886402}

...
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Development

```bash
make swag       # Generate swagger docs
make swag-fmt   # Format swagger annotations
make build      # Build binary
make test       # Run tests
make run        # Run server locally
```