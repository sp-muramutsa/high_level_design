# EdgeLink | Distributed URL Shortener & System Design Lab

An educational, production-grade URL Shortener built in **Go** and **React + TypeScript**, engineered to demonstrate the real-world distributed system patterns taught in senior systems design interviews.

---

## Architecture Overview

```
                                [ Client / wrk / Browser ]
                                             |
                                             v
                           [ Nginx Reverse Proxy & Load Balancer ]
                                         (Port 80)
                                         /       \
                      /api/ & /{code}   /         \   /* (Static Assets)
                                       /           \
                                      v             v
                          +-----------------------+ +---------------------+
                          | Go Replicas (Round-R) | | React + TS Frontend |
                          | • app1:8080           | | • Shortener Studio  |
                          | • app2:8080           | | • Live Analytics    |
                          +-----------------------+ | • Stampede Lab      |
                                  |       \         +---------------------+
                                  |        \
       +--------------------------+         +--------------------------+
       |                                                               |
       v                                                               v
[ Redis Cluster / Node ]                                     [ PostgreSQL Database ]
• ID Range Allocator (`INCRBY 1000`)                         • `urls` (UNIQUE `short_code`)
• Two-Tier Cache (Tier 2: Redis)                             • `click_events` (Partitioned)
• Click Queue (`LPUSH url_shortener:click_queue`)            • Auto-initialized via DDL
       |                                                               ^
       +---------------- (Async Batch Flusher Worker) -----------------+
```

---

## Key Architectural Decisions

### 1. Collision-Free Base62 & ID Range Allocator
* **The Problem**: Hash + truncate (e.g. MD5/SHA truncated to 7 chars) suffers from hash collisions, requiring slow DB existence checks before every write and handling retry storms.
* **The Solution**: A central atomic counter in Redis hands out chunks of 1,000 IDs (`INCRBY 1000`) to `app1` and `app2`. Each Go replica allocates sequential IDs locally in memory (`<1µs`) using `atomic.AddUint64` and converts them to 7-character Base62 (`[0-9a-zA-Z]`).
* **Guarantee**: Zero database reads before insert, zero cross-server write collisions.

### 2. Cache Stampede & Hot-Key Defense (Go `singleflight`)
* **The Problem**: A celebrity link (e.g., `bit.ly/superbowl`) suddenly receives 50,000 req/s. When its Redis cache entry expires at that exact second, all 50,000 requests miss the cache simultaneously, crushing the PostgreSQL database.
* **The Solution**: 
  1. **Tier 1 (In-Memory LRU)**: A local in-process cache (5-second TTL) inside each Go container absorbs bursts without network hops to Redis.
  2. **Go `singleflight.Group`**: When both caches miss, `singleflight.Group` coalesces concurrent goroutines so **exactly 1 query** touches PostgreSQL. The other 49,999 goroutines wait and receive the shared result.

### 3. HTTP 302 Found vs 301 Moved Permanently
* Returns **`302 Found`** with `Cache-Control: no-cache, no-store`.
* *Why?* A `301 Moved Permanently` causes client web browsers to cache the destination locally, completely blinding your server to subsequent clicks and preventing link revocation or expiration.

### 4. Decoupled Asynchronous Analytics Pipeline
* Database writes are strictly prohibited on the redirect path.
* The redirect handler pushes a lightweight event (`short_code`, `timestamp`, `ip`, `user_agent`, `referrer`) to a Redis list (`LPUSH`) in $< 1\text{ ms}$.
* A background Go worker polls the queue and flushes batches of 100 events into PostgreSQL using single-transaction batch inserts (`pgx.Batch`).

---

## Project Structure

```text
.
├── cmd/
│   └── server/
│       └── main.go               # Server initialization & graceful shutdown
├── internal/
│   ├── analytics/
│   │   └── worker.go             # Redis queue consumer & PostgreSQL batch flusher
│   ├── api/
│   │   ├── handler.go            # 302 redirects, URL creation, stampede simulation
│   │   ├── handler_test.go       # Comprehensive HTTP integration tests
│   │   └── router.go             # Chi HTTP router & CORS middleware
│   ├── cache/
│   │   ├── lru.go                # Tier-1 thread-safe in-memory LRU with TTL
│   │   ├── twotier.go            # Two-tier cache + Go singleflight engine
│   │   └── twotier_test.go       # 100-goroutine stampede verification test
│   ├── config/
│   │   └── config.go             # Environment variable loader
│   ├── generator/
│   │   ├── allocator.go          # Redis-backed atomic range allocator
│   │   ├── base62.go             # Base62 encoder & decoder
│   │   └── base62_test.go        # Roundtrip & boundary tests
│   └── storage/
│       └── postgres.go           # Connection pooling & unique constraint handling
├── migrations/
│   └── 000001_init_schema.sql    # PostgreSQL DDL with unique & partial indexes
├── docker/
│   ├── Dockerfile.backend        # Multi-stage lightweight Go container build
│   └── nginx.conf                # Load balancer & reverse proxy config
├── web/                          # React + TypeScript + Vite + Tailwind CSS frontend
│   ├── src/
│   │   ├── App.tsx               # Interactive Shortener, Live Telemetry, & Stampede Lab
│   │   └── main.tsx
│   └── package.json
├── test/
│   └── verify_cluster.sh         # End-to-end cluster integration script
├── docker-compose.yml            # 5-node cluster topology
├── go.mod
└── go.sum
```

---

## How to Run

### Option A: Turnkey Multi-Container Cluster (Docker Compose)

1. Make sure Docker Desktop is running on your machine.
2. Build and start the cluster:
   ```bash
   docker compose up --build
   ```
3. Open your browser:
   * **Web UI & Interactive Dashboard**: [http://localhost](http://localhost)
   * **Health Check**: [http://localhost/health](http://localhost/health)
   * **PostgreSQL**: `localhost:5432` (`postgres`/`postgres`)
   * **Redis**: `localhost:6379`

4. Run the automated cluster verification test:
   ```bash
   ./test/verify_cluster.sh
   ```

---

### Option B: Local Development (Without Containers)

1. **Run Go Tests**:
   ```bash
   go test -v ./...
   ```
2. **Start Backend**:
   ```bash
   go run ./cmd/server
   ```
3. **Start React Frontend**:
   ```bash
   cd web
   npm install
   npm run dev
   ```
   Open [http://localhost:3000](http://localhost:3000).
