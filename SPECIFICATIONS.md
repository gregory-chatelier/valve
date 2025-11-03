# 🧩 `valve` — Control the Flow of Data in Pipelines

## Overview

`valve` is a lightweight, cross-platform CLI tool written in Go to **control the rate of data flow** in Unix pipelines.
 It acts as a “valve” in a data stream — pacing output to a specified rate while maintaining smooth throughput and respecting backpressure.

>   “Everything is a pipe. Every pipe needs a valve.”

Typical uses:

-   Slow down verbose logs or command output for readability.
-   Throttle API requests or file transfers.
-   Limit data ingestion rates in production pipelines.
-   Simulate network conditions or gradual data arrival in testing.

------

## ✨ Design Philosophy

`valve` follows the **Unix philosophy** — it reads from `stdin`, writes to `stdout`, and does one thing well:
 **regulate flow**.

The CLI interface favors:

-   Minimalism — a few powerful flags.
-   Natural expressiveness — `--rate` behaves like human language (“10/s”, “5MB/s”).
-   Safe defaults — blocking and lossless unless told otherwise.
-   Composability — works in any shell pipeline.

------

## ⚙️ Command Reference

### Usage

```
valve [OPTIONS]
```

### Options

| Option               | Description                                                  |
| -------------------- | ------------------------------------------------------------ |
| `--rate RATE`        | Flow rate (e.g. `10/s`, `200/mn`, `5MB/s`, `2GB/h`). Intelligent parsing of units and time base. |
| `--burst COUNT`      | Number of units allowed to pass instantly before pacing resumes (token bucket). Default: 1. |
| `--jitter PERCENT`   | Adds random variation (±%) to interval timing to reduce synchronization spikes. Example: `--jitter 10` for ±10%. |
| `--progress`         | Show a dynamic progress bar and current rate to stderr.      |
| `--max-buffer SIZE`  | Maximum internal buffer (e.g. `100MB`, `1GB`). Default: `1GB`. |
| `--on-full STRATEGY` | Strategy when buffer is full: `block` (default), `drop-oldest`, `drop-newest`. |
| `--version`          | Print version info.                                          |
| `--help`             | Print help message.                                          |

------

## 🧮 Core Concepts Explained

### 🧠 `--rate` — Unified Intelligent Rate Flag

`--rate` defines the flow limit using a **human-friendly string** that combines **quantity** and **time base**.

Examples:

| Value    | Meaning                     |
| -------- | --------------------------- |
| `10/s`   | 10 items (lines) per second |
| `100/mn` | 100 items per minute        |
| `5MB/s`  | 5 megabytes per second      |
| `2GB/h`  | 2 gigabytes per hour        |
| `50k/s`  | 50 kilobytes per second     |

#### Parsing Rules

1.  If the rate unit includes `B`, `k`, `M`, or `G`, treat as **byte-based flow**.
2.  If not, treat as **line-based flow** (each line = one item).
3.  Time units: `/s`, `/mn`, `/h` (seconds, minutes, hours).
4.  Internally normalized to “units per second”.

#### Examples

```
cat bigfile.txt | valve --rate 10/s        # 10 lines/sec
cat binary.dat   | valve --rate 5MB/s      # 5 megabytes/sec
cat file | valve --rate 100/mn             # 100 lines per minute
```

This single flag replaces separate `--bytes` and `--lines` modes, simplifying UX.

------

### 🚀 `--burst` — Controlled Flexibility

Implements a **token bucket**.
 Allows short bursts of data up to `COUNT` units before throttling resumes.

-   Useful when output can tolerate temporary bursts.
-   Prevents “jerky” flow when input is chunked irregularly.

Example:

```
cat logs.txt | valve --rate 100/s --burst 20
```

→ Up to 20 lines may flow instantly, then pacing continues at 100/s average.

------

### 🎲 `--jitter` — Randomized Delay Variation

Adds randomness (±%) to pacing intervals to **avoid synchronization spikes**
 when multiple instances of `valve` or clients start simultaneously.

-   Helps prevent **“thundering herd”** effects in distributed systems.
-   Makes synthetic load testing more realistic.

Example:

```
cat ids.txt | valve --rate 50/s --jitter 15
```

→ Emits ~50 lines/s with ±15% variance in timing.

------

### 💾 Buffering & Backpressure

Internally, `valve` maintains a bounded buffer between the reader and writer goroutines.

| Flag                 | Description                              |
| -------------------- | ---------------------------------------- |
| `--max-buffer SIZE`  | Memory buffer capacity (default: `1GB`). |
| `--on-full STRATEGY` | Action when buffer is full.              |

#### Strategies

-   `block` (default): Pause reads until space frees (safe, lossless).
-   `drop-oldest`: Discard oldest data (freshest data prioritized).
-   `drop-newest`: Skip newest incoming data when full (preserve history).

Example:

```
sensor_stream | valve --rate 1000/s --on-full drop-oldest
```

------

## 🧑‍💻 Examples

### Basic Usage

```
cat file.txt | valve --rate 10/s
```

→ Emit 10 lines per second.

```
cat bigfile.bin | valve --rate 5MB/s --progress
```

→ Stream binary file at 5 MB/s with live progress display.

------

### With Burst and Jitter

```
cat events.txt | valve --rate 200/s --burst 50 --jitter 10
```

→ Allow short bursts (50 lines), then smooth pacing around 200/s ±10%.

------

### Using in Shell Scripts

#### API Request Loop

```
#!/bin/bash
cat user_ids.txt | valve --rate 5/s | while read id; do
  curl -s "https://api.example.com/users/$id" >> users.json
done
```

→ Limits API requests to 5 per second.

------

#### Controlled Log Playback

```
#!/bin/bash
valve --rate 20/s < app.log | while read line; do
  echo "$(date +%H:%M:%S) $line"
done
```

→ Replays logs at 20 lines per second.

------

#### Throttled Data Migration

```
#!/bin/bash
pg_dump source_db | valve --rate 10MB/s --progress | psql target_db
```

→ Keeps migration speed within safe DB limits.

------

#### IoT Data Ingestion

```
#!/bin/bash
mosquitto_sub -t sensors/# | valve --rate 5k/s --on-full drop-oldest | process_sensors
```

→ Limits incoming sensor data to 5 KB/s and drops old data on overload.

------

### Combine with Other Unix Tools

```
tail -f /var/log/syslog | valve --rate 5/s
```

→ Make live logs readable at human speed.

```
find . -type f | valve --rate 10/s | xargs -n1 rm
```

→ Delete files steadily instead of instant flood.

------

## 🧱 Technical Notes for Implementation (Go)

-   **Concurrency model**:
    -   Reader goroutine → buffered channel → Writer goroutine.
-   **Rate limiter**:
    -   Token bucket or leaky bucket algorithm.
    -   Supports fractional tokens for smooth pacing.
-   **Buffering**:
    -   `chan []byte` with capacity based on `--max-buffer`.
    -   Behavior on full channel depends on `--on-full`.
-   **Jitter**:
    -   Random factor on sleep duration (uniform ±PERCENT).
-   **Progress**:
    -   Print metrics (lines, bytes, rate) to stderr periodically.
-   **Exit codes**:
    -   Pass through from downstream unless broken pipe detected.

------

## 🧭 Example Help Output

```
valve - control the flow of data in pipelines

Usage:
  valve [OPTIONS]

Options:
  --rate RATE          Flow rate (e.g., 10/s, 200/mn, 5MB/s, 2GB/h)
  --burst COUNT        Burst size before pacing resumes [default: 1]
  --jitter PERCENT     Add ±% random timing variation
  --progress           Show progress bar and live rate
  --max-buffer SIZE    Maximum internal buffer [default: 1GB]
  --on-full STRATEGY   On buffer full: block, drop-oldest, drop-newest [default: block]
  --version            Show version info
  --help               Show this help message

Examples:
  cat file.txt | valve --rate 10/s
  cat file.txt | valve --rate 1MB/s --progress
  tail -f log | valve --rate 5/s --jitter 10
```

------

## ✅ MVP Feature Checklist

| Feature                              | Description                       | Status |
| ------------------------------------ | --------------------------------- | ------ |
| Core line & byte-based rate limiting | Single `--rate` flag with parsing | ✅      |
| Token bucket (`--burst`)             | Controlled bursts                 | ✅      |
| Jittered pacing (`--jitter`)         | Randomized variation              | ✅      |
| Bounded buffer with strategies       | `--max-buffer`, `--on-full`       | ✅      |
| Progress output                      | Optional TUI progress bar         | ✅      |
| Safe defaults                        | Block + lossless by default       | ✅      |

------

## 📦 Example README Tagline

>   **Valve** — A smart CLI to control the flow of data in Unix pipelines.
>    Throttle logs, APIs, or file streams at any rate — safely, smoothly, and simply.









More advanced practical examples :



## API Call Throttling (Avoiding Bans)

**Problem:** APIs often limit to *N requests per second/minute*.
 **Old way:**

```
# Hacky sleep approach
for user in $(cat users.txt); do
  curl -s "https://api.example.com/users/$user"
  sleep 0.5   # 2 req/sec — inaccurate and slow
done
```

**With `flowctl`:**

```
# Smooth, precise, non-blocking
cat users.txt | flowctl --rate 2/s | while read user; do
  curl -s "https://api.example.com/users/$user"
done
```

✅ No busy waiting, no rate bursts, easy to adjust dynamically. 

## Controlled File Processing Loop

**Problem:** You have a directory of files to process, but you want to throttle CPU usage.
 **With `flowctl`:**

```
find /data/images -type f | flowctl --rate 5/s | while read img; do
  ./compress_image "$img" &
done
wait
```

✅ Avoids CPU spikes or filesystem thrashing.

------

## Staged Notifications or Email Sending

**Problem:** Avoid being rate-limited by SMTP servers.
 **With `flowctl`:**

```
cat recipients.txt | flowctl --rate 200/hour | while read email; do
  sendmail "$email" < template.txt
done
```

✅ Keeps email sending safe, smooth, and compliant.

### **CI Stress Testing or Backpressure Experiments**

When validating pipeline robustness or service reliability, `valve` helps you **control load pressure** precisely.

```
yes "test message" | valve --rate 1k/s --jitter 5 | ./consumer_test
```

→ Simulates 1,000 messages per second with 5% random jitter.

