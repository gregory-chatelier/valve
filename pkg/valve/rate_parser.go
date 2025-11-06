package valve

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseStrategy parses a strategy string and returns the corresponding Strategy type.
func ParseStrategy(s string) (Strategy, error) {
	switch s {
	case "block":
		return Block, nil
	case "drop-newest":
		return DropNewest, nil
	default:
		return "", fmt.Errorf("invalid strategy: %s", s)
	}
}

// ParseRate parses a human-friendly rate string (e.g., "10/s", "5MB/s", "100/mn")
// and returns the rate in items/bytes per second and a boolean indicating if it's byte-based.
func ParseRate(rateStr string) (float64, bool, error) {
	rateStr = strings.ToLower(strings.TrimSpace(rateStr))

	parts := strings.Split(rateStr, "/")
	if len(parts) != 2 {
		return 0, false, fmt.Errorf("invalid rate format: %s", rateStr)
	}

	quantityStr := parts[0]
	timeUnitStr := parts[1]

	var unitStr string
	for i, r := range quantityStr {
		if r >= '0' && r <= '9' || r == '.' {
			continue
		} else {
			unitStr = quantityStr[i:]
			quantityStr = quantityStr[:i]
			break
		}
	}

	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid quantity in rate: %w", err)
	}

	if quantity < 0 {
		return 0, false, fmt.Errorf("rate quantity cannot be negative")
	}

	isBytes := false
	multiplier := 1.0

	switch unitStr {
	case "b":
		isBytes = true
	case "k", "kb":
		isBytes = true
		multiplier = 1024.0
	case "m", "mb":
		isBytes = true
		multiplier = 1024.0 * 1024.0
	case "g", "gb":
		isBytes = true
		multiplier = 1024.0 * 1024.0 * 1024.0
	case "":
		// Line-based, no unit specified
	default:
		return 0, false, fmt.Errorf("unknown unit: %s", unitStr)
	}

	var timeDivisor float64
	switch timeUnitStr {
	case "s":
		timeDivisor = 1.0
	case "m", "mn":
		timeDivisor = 60.0
	case "h":
		timeDivisor = 3600.0
	default:
		return 0, false, fmt.Errorf("unknown time unit: %s", timeUnitStr)
	}

	return (quantity * multiplier) / timeDivisor, isBytes, nil
}
