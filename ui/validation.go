package ui

import (
	"fmt"
	"strconv"
)

// parseIntOrDefault attempts to parse a string as an integer.
// Returns the parsed value or defaultValue if parsing fails.
func parseIntOrDefault(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return val
}

// parseIntInRange parses a string as an integer and validates it's within the given range.
// Returns the parsed value, or an error if parsing fails or value is out of range.
func parseIntInRange(s string, min, max int, fieldName string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("%s cannot be empty", fieldName)
	}

	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid number", fieldName)
	}

	if val < min || val > max {
		return 0, fmt.Errorf("%s must be between %d and %d", fieldName, min, max)
	}

	return val, nil
}

// parsePort parses a port number from a string.
// Returns the port number or defaultPort if parsing fails or port is invalid.
func parsePort(s string, defaultPort int) int {
	if s == "" {
		return defaultPort
	}

	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return defaultPort
	}

	return port
}

// validatePort validates a port string and returns an error if invalid.
func validatePort(s string) error {
	if s == "" {
		return fmt.Errorf("port cannot be empty")
	}

	port, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("port must be a valid number")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return nil
}
