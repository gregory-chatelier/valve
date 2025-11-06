package valve

import (
	"fmt"
)

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