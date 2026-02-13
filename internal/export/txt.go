package export

import (
	"fmt"
	"os"
	"strings"

	"iperf-tool/internal/format"
	"iperf-tool/internal/model"
)

// WriteTXT writes test results to a text file using formatted output.
func WriteTXT(path string, results []model.TestResult) error {
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(format.FormatResult(&r))
	}
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("write txt file: %w", err)
	}
	return nil
}
