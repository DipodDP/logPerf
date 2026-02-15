package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SavedFilesList displays a list of saved result files from disk
type SavedFilesList struct {
	mu        sync.Mutex
	files     []FileInfo
	list      *widget.List
	container *fyne.Container
}

// FileInfo holds metadata about a saved file
type FileInfo struct {
	Name     string
	Path     string
	Size     int64
	Modified time.Time
}

// NewSavedFilesList creates a new saved files list component
func NewSavedFilesList() *SavedFilesList {
	sfl := &SavedFilesList{
		files: []FileInfo{},
	}

	// Create list widget
	sfl.list = widget.NewList(
		func() int {
			sfl.mu.Lock()
			defer sfl.mu.Unlock()
			return len(sfl.files)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			sfl.mu.Lock()
			defer sfl.mu.Unlock()
			if id >= len(sfl.files) {
				return
			}
			label := obj.(*widget.Label)
			label.SetText(sfl.formatFileItem(sfl.files[id]))
		},
	)

	// Handle double-click to open file
	sfl.list.OnSelected = func(id widget.ListItemID) {
		sfl.mu.Lock()
		if id >= len(sfl.files) {
			sfl.mu.Unlock()
			return
		}
		path := sfl.files[id].Path
		sfl.mu.Unlock()

		// Open file in system default application
		go sfl.openFile(path)

		// Deselect immediately to allow re-selection
		sfl.list.UnselectAll()
	}

	// Create header and container
	header := widget.NewLabel("Saved Results")
	header.TextStyle = fyne.TextStyle{Bold: true}

	sfl.container = container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		nil, nil, nil,
		sfl.list,
	)

	// Initial scan
	sfl.Refresh()

	return sfl
}

// Container returns the container widget
func (sfl *SavedFilesList) Container() *fyne.Container {
	return sfl.container
}

// Refresh rescans the directory and updates the file list
func (sfl *SavedFilesList) Refresh() {
	files, err := sfl.scanFiles()
	if err != nil {
		// Log error but don't crash
		fmt.Fprintf(os.Stderr, "Error scanning files: %v\n", err)
		return
	}

	sfl.mu.Lock()
	sfl.files = files
	sfl.mu.Unlock()

	sfl.list.Refresh()
}

// scanFiles discovers result files in the working directory
func (sfl *SavedFilesList) scanFiles() ([]FileInfo, error) {
	var files []FileInfo

	// Find CSV files
	csvFiles, err := filepath.Glob("*.csv")
	if err != nil {
		return nil, err
	}

	// Find TXT files
	txtFiles, err := filepath.Glob("*.txt")
	if err != nil {
		return nil, err
	}

	// Combine and get file info
	allFiles := append(csvFiles, txtFiles...)
	for _, filename := range allFiles {
		info, err := os.Stat(filename)
		if err != nil {
			continue // Skip files we can't stat
		}

		files = append(files, FileInfo{
			Name:     filename,
			Path:     filename,
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}

	// Sort by modified time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified.After(files[j].Modified)
	})

	return files, nil
}

// formatFileItem formats a file entry for display
func (sfl *SavedFilesList) formatFileItem(fi FileInfo) string {
	// Format size
	var sizeStr string
	if fi.Size < 1024 {
		sizeStr = fmt.Sprintf("%d B", fi.Size)
	} else if fi.Size < 1024*1024 {
		sizeStr = fmt.Sprintf("%.1f KB", float64(fi.Size)/1024)
	} else {
		sizeStr = fmt.Sprintf("%.1f MB", float64(fi.Size)/(1024*1024))
	}

	// Format time (show date if not today, time if today)
	now := time.Now()
	var timeStr string
	if fi.Modified.Year() == now.Year() && fi.Modified.YearDay() == now.YearDay() {
		timeStr = fi.Modified.Format("15:04:05")
	} else {
		timeStr = fi.Modified.Format("2006-01-02")
	}

	return fmt.Sprintf("%s  (%s, %s)", fi.Name, sizeStr, timeStr)
}

// openFile opens a file with the system default application
func (sfl *SavedFilesList) openFile(path string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported platform for opening files: %s\n", runtime.GOOS)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", path, err)
	}
}
