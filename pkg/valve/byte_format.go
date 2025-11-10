package valve

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// parseByteSize converts a string like "1MB", "512KB", "1.5GB" into bytes
func ParseByteSize(size string) (int, error) {
	re := regexp.MustCompile(`(?i)^(\d*\.?\d+)\s*(B|K|KB|M|MB|G|GB)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(size))
	if matches == nil {
		return 0, fmt.Errorf("invalid size format: %s (expected format: number followed by B, K, KB, M, MB, G, or GB)", size)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", matches[1])
	}

	const (
		KB            = 1024
		MB            = KB * 1024
		GB            = MB * 1024
		MaxBufferSize = 5 * MB // maximum buffer size
		MinBufferSize = 4 * KB // minimum buffer size (4KB)
	)

	var bytes float64
	switch strings.ToUpper(matches[2]) {
	case "B":
		bytes = value
	case "K", "KB":
		bytes = value * KB
	case "M", "MB":
		bytes = value * MB
	case "G", "GB":
		bytes = value * GB
	}

	if bytes > MaxBufferSize {
		return 0, fmt.Errorf("size too large: maximum allowed is %s", formatBytes(MaxBufferSize))
	}

	if bytes < MinBufferSize {
		return 0, fmt.Errorf("size too small: minimum allowed is %s", formatBytes(MinBufferSize))
	}

	return int(bytes), nil
}

// formatBytes converts a float64 byte count into a human-readable string (e.g., 10.5 MB).
func formatBytes(bytes float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", bytes/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", bytes/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", bytes/KB)
	default:
		return fmt.Sprintf("%.2f B", bytes)
	}
}
