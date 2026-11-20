# High-Level Design (HLD) & Distributed Systems Labs

A collection of production-grade reference implementations and educational labs demonstrating core distributed systems patterns and high-scale architecture.

---

## Projects in this Repository

| Project | Description | Stack | Highlights |
| :--- | :--- | :--- | :--- |
| [**url-shortner**](./url-shortner) | Distributed URL Shortener & System Design Lab | Go, Redis, PostgreSQL, Nginx, React + TS | • Collision-free Base62 ID range allocation<br>• Two-tier caching (In-process LRU + Redis)<br>• Cache stampede prevention via Go `singleflight`<br>• Decoupled asynchronous click stream<br>• Interactive React dashboard & stampede simulator |

---

## Running the Projects

Each project includes its own independent Docker Compose cluster, unit test suite, and educational dashboard. See the individual project READMEs for detailed setup guides:

* [**URL Shortener Documentation**](./url-shortner/README.md)
