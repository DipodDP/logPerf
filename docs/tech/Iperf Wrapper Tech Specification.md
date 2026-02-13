# Rationale

I want you to perform a **deep technical planning** for creating a **Golang utility** that provides a **Graphical User Interface (GUI)** for running and managing **iperf** network performance tests, collecting results, and exporting them for analysis in Excel.

**Core Requirements:**

1. **Overview & Architecture**
   * Recommend a suitable **GUI framework** for Go (cross-platform Windows/Linux/macOS).
   * Define the overall architecture: frontend GUI, backend logic, iperf command execution, result parsing, storage/export.
   * Discuss pros/cons of using Go for GUI and command control.
2. **iperf Integration**
   * Explain how to execute iperf commands (client and server) from Go.
   * Provide patterns for running iperf as a subprocess, capturing stdout/stderr, and parsing results.
   * Example iperf command for Windows:

     ```
     .\iperf.exe -c 192.168.222.101 -p 4321 -P 4 -i 1 -t 100
     ```

   * Describe how to expose **configurable parameters** (server, port, parallel streams, interval, time, etc.) in the GUI and map to iperf flags.
3. **Remote Control of iperf Server**
   * Research methods for controlling a remote iperf server:
     * Running iperf server via SSH.
     * Starting, stopping, querying status.
     * Handling authentication (keys vs password).
     * Cross-platform remote execution concerns.
   * Assess libraries in Go for SSH (e.g., `golang.org/x/crypto/ssh` or wrappers).
4. **Data Logging & Storage**
   * Recommend how to capture results (TXT/CSV) for future analysis.
   * Define a format/schema for exported logs.
   * Provide a flow for saving periodically and exporting to Excel-friendly format.
   * Discuss using built-in Go CSV/Excel libraries (e.g., `encoding/csv`, `excelize`).
5. **GUI Functionality**
   * Propose necessary UI components:
     * Parameter form
     * Start/Stop buttons
     * Live output area
     * History/results table
     * Remote control section
     * Export logs button
   * Provide mock layout or component list with behavior.
6. **User Experience**
   * Explain options for:
     * Selecting remote machines
     * Saving presets
     * Error handling and messages
   * Describe validation and responsiveness.
7. **Concurrency & Process Control**
   * Detail how to manage multiple iperf runs concurrently.
   * How to update GUI real-time from background tasks safely in Go.
8. **Security Considerations**
   * Address safe handling of remote credentials.
   * Secure SSH practices.
   * Restricting unauthorized control.
9. **Testing Strategy**
   * How to test the tool (unit, integration, UI tests).
   * Simulating network conditions.
10. **Deliverables**
    * Step-by-step development plan, including milestones.
    * Example code snippets for key parts (running iperf, parsing, GUI launch).
