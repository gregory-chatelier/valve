# `valve` — Control the data flow in pipelines

## Overview

`valve` is a lightweight CLI tool for controlling the rate of data flow in your pipelines.
 It acts as a “valve” in a data stream — pacing output to a specified rate while maintaining smooth throughput and respecting back-pressure.

>   “Everything is a pipe. Every pipe needs a valve.”

`valve` follows the **Unix philosophy** — it reads from `stdin`, writes to `stdout`, and does one thing well:  **regulate flow**.



What started as a tool to make streaming LLM output readable at human speed has become a versatile utility for managing data streams. Use it to keep API requests within a budget, throttle logs, or control resource consumption in data-intensive scripts at any rate — safely, smoothly, and simply.

## Installation

This command will download and install `valve` to a standard location for your system.

**Recommended (User-level):**
Installs to `$HOME/.local/bin` (Linux/macOS) or a user-specific `bin` directory (Windows).
```bash
curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh | sh
```

**System-wide (Requires `sudo`):**
Installs to `/usr/local/bin`.
```bash
sudo curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh | sh
```

**Custom Directory:**
Use the `INSTALL_DIR` environment variable to specify a custom path.
```bash
curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh | INSTALL_DIR=$HOME/bin sh
```

## Usage

The basic structure is to pipe data through `valve`:

```bash
cat data.txt | valve [OPTIONS] | consumer_command
```

### Examples

#### 1. Make live logs readable
Stream a log file at a comfortable reading speed of 5 lines per second.

```bash
tail -f /var/log/syslog | valve --rate 5/s
```

#### 2. Throttle API calls
Most APIs have rate limits. Instead of adding `sleep` commands to your loops, you can precisely control the request rate and your budget.

```bash
# Smooth, precise, non-blocking
cat user_ids.txt | valve --rate 2/s | while read id; do
  curl -s "https://api.example.com/users/$id"
done
```

#### 3. Control data transfer speed
Transfer a large file at a maximum of 10MB per second and monitor the progress.

```bash
cat database.sql | valve --rate 10MB/s --progress | psql target_db
```

#### 4. Smooth out batch processes
Evenly space 200 emails over one hour to avoid being flagged as spam. `valve` handles the timing automatically.

```bash
cat recipients.txt | valve --rate 200/h | while read email; do
  sendmail "$email" < template.txt
done
```

You have a bunch of files to process, but you want to throttle CPU usage

```bash
find /data/images -type f | valve --rate 5/s | while read img; do
  ./compress_image "$img" &
done
```

## Command Reference

| Option | Description |
| :--- | :--- |
| `--rate RATE` | Flow rate (e.g. `10/s`, `200/mn`, `5MB/s`, `2GB/h`). |
| `--burst COUNT` | Number of items to allow in an initial burst. Default: `1`. |
| `--jitter PERCENT` | Adds ±% random variation to timing (e.g., `--jitter 10`). |
| `--progress` | Show a live progress indicator on `stderr`. |
| `--max-buffer SIZE` | Maximum internal buffer size (e.g., `64KB`, `128KB`, `512KB`). Default: `128KB`. Maximum allowed: `5MB`. |
| `--on-full STRATEGY` | What to do when the buffer is full: `block` (default), `drop-newest`. |
| `--version` | Print version info. |

### Advanced Features

*   **`--burst`**: Implements a token bucket, allowing a short burst of data to pass through before the rate limit kicks in. Useful for handling irregular input without jerky flow.
*   **`--jitter`**: Adds randomness to the pacing interval. This helps prevent the "thundering herd" problem in distributed systems where multiple processes might otherwise synchronize and create load spikes.
*   **`--on-full`**: Defines the backpressure strategy.
    *   `block` (default): Pauses reading from `stdin` until the buffer has space. This is the safest, lossless option.
    *   `drop-newest`: When the buffer is full, new incoming items are ignored. Good for preserving a backlog of historical data.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
