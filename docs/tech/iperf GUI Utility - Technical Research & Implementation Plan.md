# Golang _iperf3_ GUI Utility: Design & Implementation Plan

## Overview & Architecture

We recommend using a Go-native or Go-friendly GUI toolkit that is truly cross-platform (Windows, macOS, Linux). Candidates include **Fyne**, **Gio**, and **Wails**:

- **Fyne** (pure Go, OpenGL-based) lets you “build native apps that work everywhere”. It’s easy to install and supports desktop/mobile. Its drawback is that some developers find its mobile-oriented layout system inflexible for complex desktop UIs.
- **Gio** (immediate-mode Go UI) supports all major platforms (desktop, mobile, WebAssembly). It has many widgets and is very active (used in projects like Tailscale), but it requires more boilerplate and has limited documentation.
- **Wails** (Go backend + HTML/JS frontend) is a hybrid approach: you write the GUI in web languages (HTML, CSS, JavaScript) and bind it to Go logic. Wails allows full-fledged, modern UIs using any web framework; it’s fast to develop if you have web skills. Its trade-off is a larger binary (includes WebView) and the need to know web technologies.
Alternatively, one can use native bindings (e.g. Qt via therecipe/qt or GTK via gotk3) or the IUP toolkit (via [gen2brain/iup-go]) for a native look. For example, IUP provides native widgets on Windows/macOS (GTK on Linux) and is mature, but its API is string-based and it has poor concurrency support. We summarize pros/cons:
|**Framework**|**Approach**|**Pros**|**Cons**|
|---|---|---|---|
|**Fyne**|Pure Go (OpenGL)|Native look, single-codebase for desktop/mobile|Layout constraints, mobile-centric design|
|**Gio**|Pure Go (Immediate)|Cross-platform (including mobile/WebAssembly), many widgets|Steep learning curve, lacks docs|
|**Wails**|Go + HTML/JS (WebView)|Modern UI via HTML/CSS/JS, lots of web libraries|Requires web development (JS/HTML) skills|
|**IUP (gen2brain)**|C toolkit (via Go binding)|Native OS controls, long history|String-based API, poor concurrency|
|**Qt via binding**|C++ (via CGO)|Rich feature set, designer tools|Heavy CGO deps, large binary, GPL/LGPL licensing concerns|
Based on these, **Fyne** or **Gio** are strong candidates if we want a pure-Go solution; **Wails** is appealing if we prefer rapid UI prototyping via web tech. The final choice depends on developer expertise (Go-only vs. Go+web) and required look/feel.
**Architecture:** The application should separate the GUI front-end from the backend logic. A typical architecture:
- **Frontend (GUI):** The window/UI that gathers user inputs (server IPs, ports, test parameters) and displays outputs (live logs, charts, history).
- **Backend Logic:** Goroutines and handlers that launch _iperf3_ processes, parse their output, and record results. Use Go channels or event queues to pass progress/errors back to the GUI thread safely.
- **iperf3 Process Execution:** A component that constructs the _iperf3_ command-line (client or server) from the user parameters, then runs it via `os/exec`. It captures `stdout`/`stderr` and signals completion or errors.
- **Result Parser:** Since _iperf3_ supports JSON output, parse the JSON into Go structs for analysis. This makes extracting metrics (bandwidth, bytes, etc.) easy.
- **Data Storage/Export:** After each test, format the data into CSV rows or an Excel worksheet. Store recent results in memory (and optionally in files) for display. On user request, export all logs to CSV or XLSX.
This modular design lets the GUI simply call a “start test” routine without worrying about parsing, and lets future extensions (e.g. persisting to a database) be added under the backend. Using Go’s concurrency (goroutines + channels) we can run multiple tests (e.g. to different servers) in parallel without freezing the UI.

------------------------------------------------------------------------

### High-Level Architecture

    +----------------------+
    |        GUI           |
    | (Fyne / Wails / Gio) |
    +----------+-----------+
               |
               v
    +----------------------+
    |    Application Core  |
    |  - Test Manager      |
    |  - SSH Manager       |
    |  - Logger            |
    |  - Exporter          |
    +----------+-----------+
               |
               v
    +----------------------+
    |   iperf3 Subprocess  |
    |  (Client / Server)   |
    +----------------------+

### Modules

1.  GUI Layer
2.  Command Execution Layer
3.  Result Parser
4.  Storage Layer
5.  Export Layer (CSV/XLSX)
6.  SSH Remote Control Layer

------------------------------------------------------------------------

## 2. iperf3 Integration

### Example Command

    iperf3 -c 192.168.222.101 -p 4321 -P 4 -i 1 -t 100 -J

### Running iperf3 in Go

``` go
ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
defer cancel()
  
cmd := exec.CommandContext(ctx,
    "iperf3",
    "-c", "192.168.222.101",
    "-p", "4321",
    "-P", "4",
    "-i", "1",
    "-t", "100",
    "-J",
)
  
var stdout bytes.Buffer
var stderr bytes.Buffer
  
cmd.Stdout = &stdout
cmd.Stderr = &stderr
  
if err := cmd.Run(); err != nil {
    log.Printf("Error: %v, stderr: %s", err, stderr.String())
    return
}
  
var result map[string]interface{}
json.Unmarshal(stdout.Bytes(), &result)
```

------------------------------------------------------------------------

## 3. Remote iperf3 Server Control (SSH)

### Libraries

-   `golang.org/x/crypto/ssh`

### SSH Execution Example

``` go
config := &ssh.ClientConfig{
    User: "user",
    Auth: []ssh.AuthMethod{
        ssh.PublicKeys(privateKey),
    },
    HostKeyCallback: ssh.InsecureIgnoreHostKey(),
}
  
client, _ := ssh.Dial("tcp", "192.168.1.10:22", config)
session, _ := client.NewSession()
defer session.Close()
  
output, err := session.CombinedOutput("iperf3 -s -p 5201")
```

------------------------------------------------------------------------

## 4. Data Logging & Storage

### Recommended Format

Primary storage: JSON (raw iperf output) Export format: CSV
(Excel-friendly)

### CSV Export Example

``` go
file, _ := os.Create("results.csv")
writer := csv.NewWriter(file)
  
writer.Write([]string{"Time", "Sent_bps", "Received_bps"})
writer.Write([]string{"2026-02-13", "100000000", "98000000"})
  
writer.Flush()
```

------------------------------------------------------------------------

## 5. GUI Layout Design

### Required Components

-   Parameter Form
-   Start Button
-   Stop Button
-   Live Output Console
-   History Table
-   Remote Server Section
-   Export Button

### Suggested Layout

    -------------------------------------------------
    | Parameters          | Remote Server          |
    -------------------------------------------------
    | Start | Stop | Export                        |
    -------------------------------------------------
    | Live Output (scrollable console)             |
    -------------------------------------------------
    | Test History Table                           |
    -------------------------------------------------
------------------------------------------------------------------------

## 6. Concurrency Model

-   Each test runs in its own goroutine
-   Use `context.Context` for cancellation
-   Use channels for GUI-safe updates
Example:

``` go
go func() {
    runTest()
    resultChan <- result
}()
```

------------------------------------------------------------------------

## 7. Security Considerations

-   Prefer SSH key authentication
-   Never store passwords in plaintext
-   Validate host keys
-   Avoid root execution unless necessary

------------------------------------------------------------------------

## 8. Testing Strategy

### Unit Tests

-   JSON parsing
-   Command generation

### Integration Tests

-   Local iperf server/client interaction

### Network Simulation

-   Use Linux `tc/netem` for latency/loss simulation

------------------------------------------------------------------------

## 9. Development Milestones

### Phase 1 -- Core Engine

-   Implement iperf execution
-   Implement JSON parsing
-   Implement CSV export

### Phase 2 -- GUI

-   Basic form + Start/Stop
-   Live output window

### Phase 3 -- Remote Control

-   SSH connection manager
-   Start/Stop remote server

### Phase 4 -- Polish

-   Presets
-   Error dialogs
-   Cross-platform packaging

------------------------------------------------------------------------

## 10. Final Deliverables

-   Cross-platform Go GUI app
-   Local and remote iperf3 control
-   JSON logging
-   CSV export for Excel
-   Preset management
-   Safe concurrency model
