package export

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	measurementMu      sync.Mutex
	lastMeasurementTs  string
	measurementCounter int
)

// NextMeasurementID returns a unique ID of the form "YYYYMMDD-HHMMSS-NN" for
// the given timestamp. The counter resets to 01 each new second.
func NextMeasurementID(ts time.Time) string {
	measurementMu.Lock()
	defer measurementMu.Unlock()
	tsStr := ts.Format("20060102-150405")
	if tsStr == lastMeasurementTs {
		measurementCounter++
	} else {
		lastMeasurementTs = tsStr
		measurementCounter = 1
	}
	return fmt.Sprintf("%s-%02d", tsStr, measurementCounter)
}

// DateSuffix returns the date portion in "02.01.2006" format.
func DateSuffix(t time.Time) string {
	return t.Format("02.01.2006")
}

// BuildPath returns a file path of the form base + suffix + "_" + date + ext.
// Files are appended to (not duplicated) on subsequent writes, so no collision
// counter is needed.
// For interval logs, pass suffix="_log".
func BuildPath(base, suffix, ext string, t time.Time) string {
	date := DateSuffix(t)
	return fmt.Sprintf("%s%s_%s%s", base, suffix, date, ext)
}

// BuildLogPath returns a file path of the form base + suffix + ext with no date component.
// Used for append-only logs that accumulate across days (e.g. results_log.csv).
func BuildLogPath(base, suffix, ext string) string {
	return fmt.Sprintf("%s%s%s", base, suffix, ext)
}

// EnsureDir creates the directory component of path (equivalent to mkdir -p)
// with mode 0755. It is a no-op if the directory already exists.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
