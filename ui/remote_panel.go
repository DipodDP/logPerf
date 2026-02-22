package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	internalssh "iperf-tool/internal/ssh"
)

// RemotePanel provides SSH server control UI.
type RemotePanel struct {
	hostEntry     *widget.Entry
	userEntry     *widget.Entry
	keyPathEntry  *widget.Entry
	passwordEntry *widget.Entry
	portEntry     *widget.Entry

	connectBtn    *widget.Button
	disconnectBtn *widget.Button
	installBtn    *widget.Button
	startSrvBtn   *widget.Button
	stopSrvBtn    *widget.Button
	statusEntry *ReadOnlyEntry

	client    *internalssh.Client
	srvMgr    *internalssh.ServerManager
	container *fyne.Container
}

// NewRemotePanel creates the SSH remote server control panel.
func NewRemotePanel() *RemotePanel {
	rp := &RemotePanel{
		srvMgr: internalssh.NewServerManager(),
	}

	rp.hostEntry = widget.NewEntry()
	rp.hostEntry.SetPlaceHolder("192.168.1.100")

	rp.userEntry = widget.NewEntry()
	rp.userEntry.SetPlaceHolder("root")

	rp.keyPathEntry = widget.NewEntry()
	rp.keyPathEntry.SetPlaceHolder("~/.ssh/id_rsa")

	rp.passwordEntry = widget.NewPasswordEntry()
	rp.passwordEntry.SetPlaceHolder("Optional")

	rp.portEntry = widget.NewEntry()
	rp.portEntry.SetText("5201")

	rp.statusEntry = NewReadOnlyEntry()
	rp.statusEntry.MultiLine = false
	rp.statusEntry.SetText("Disconnected")

	rp.connectBtn = widget.NewButton("Connect SSH", rp.onConnect)
	rp.disconnectBtn = widget.NewButton("Disconnect SSH", rp.onDisconnect)
	rp.disconnectBtn.Disable()

	rp.installBtn = widget.NewButton("Install iperf3", rp.onInstall)
	rp.installBtn.Disable()

	rp.startSrvBtn = widget.NewButton("Start Server", rp.onStartServer)
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn = widget.NewButton("Stop Server", rp.onStopServer)
	rp.stopSrvBtn.Disable()

	connection := container.NewVBox(
		widget.NewLabel("Host"), rp.hostEntry,
		widget.NewLabel("Username"), rp.userEntry,
		widget.NewLabel("SSH Key Path"), rp.keyPathEntry,
		widget.NewLabel("Password"), rp.passwordEntry,
		container.NewHBox(rp.connectBtn, rp.disconnectBtn),
	)

	control := container.NewVBox(
		widget.NewLabel("Server Port"), rp.portEntry,
		rp.installBtn,
		container.NewHBox(rp.startSrvBtn, rp.stopSrvBtn),
	)

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("SSH Connection", connection),
		widget.NewAccordionItem("Remote Server", control),
	)
	accordion.Open(0)

	rp.container = container.NewVBox(
		accordion,
		rp.statusEntry,
	)

	return rp
}

// Container returns the panel's container.
func (rp *RemotePanel) Container() *fyne.Container {
	return rp.container
}

// LoadPreferences restores panel values from persistent preferences.
func (rp *RemotePanel) LoadPreferences(prefs fyne.Preferences) {
	if v := prefs.String("remote.host"); v != "" {
		rp.hostEntry.SetText(v)
	}
	if v := prefs.String("remote.user"); v != "" {
		rp.userEntry.SetText(v)
	}
	if v := prefs.String("remote.key_path"); v != "" {
		rp.keyPathEntry.SetText(v)
	}
	if v := prefs.String("remote.port"); v != "" {
		rp.portEntry.SetText(v)
	}
}

// SavePreferences persists panel values to preferences (excluding password).
func (rp *RemotePanel) SavePreferences(prefs fyne.Preferences) {
	prefs.SetString("remote.host", rp.hostEntry.Text)
	prefs.SetString("remote.user", rp.userEntry.Text)
	prefs.SetString("remote.key_path", rp.keyPathEntry.Text)
	prefs.SetString("remote.port", rp.portEntry.Text)
}

func (rp *RemotePanel) onConnect() {
	cfg := internalssh.ConnectConfig{
		Host:     rp.hostEntry.Text,
		Port:     22,
		User:     rp.userEntry.Text,
		KeyPath:  rp.keyPathEntry.Text,
		Password: rp.passwordEntry.Text,
	}

	rp.connectBtn.Disable()
	rp.statusEntry.SetText("Connecting...")

	go func() {
		client, err := internalssh.Connect(cfg)
		if err != nil {
			fyne.Do(func() {
				rp.statusEntry.SetText(fmt.Sprintf("Error: %v", err))
				rp.connectBtn.Enable()
			})
			return
		}

		// Check if iperf3 server is already running
		running, _ := rp.srvMgr.CheckStatus(client)

		fyne.Do(func() {
			rp.client = client
			rp.disconnectBtn.Enable()
			rp.installBtn.Enable()
			if running {
				rp.statusEntry.SetText(fmt.Sprintf("Connected to %s (server running)", cfg.Host))
				rp.stopSrvBtn.Enable()
			} else {
				rp.statusEntry.SetText(fmt.Sprintf("Connected to %s", cfg.Host))
				rp.startSrvBtn.Enable()
			}
		})
	}()
}

// RestartServer kills any stuck iperf3 processes on the remote host and
// starts a fresh server. Returns nil if no SSH connection is active.
func (rp *RemotePanel) RestartServer() error {
	if rp.client == nil {
		return fmt.Errorf("not connected via SSH")
	}

	port := rp.getPort()

	if err := rp.srvMgr.RestartServer(rp.client, port); err != nil {
		return err
	}

	fyne.Do(func() {
		rp.statusEntry.SetText(fmt.Sprintf("Server restarted on port %d", port))
		rp.startSrvBtn.Disable()
		rp.stopSrvBtn.Enable()
	})
	return nil
}

// IsConnected returns whether an SSH connection is active.
func (rp *RemotePanel) IsConnected() bool {
	return rp.client != nil
}

// Host returns the configured SSH host address.
func (rp *RemotePanel) Host() string {
	return rp.hostEntry.Text
}

// getPort returns the configured iperf3 server port, or 5201 if invalid.
func (rp *RemotePanel) getPort() int {
	return parsePort(rp.portEntry.Text, 5201)
}

func (rp *RemotePanel) onDisconnect() {
	if rp.client != nil {
		rp.client.Close()
		rp.client = nil
	}
	rp.statusEntry.SetText("Disconnected")
	rp.connectBtn.Enable()
	rp.disconnectBtn.Disable()
	rp.installBtn.Disable()
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn.Disable()
}

func (rp *RemotePanel) onStartServer() {
	if rp.client == nil {
		return
	}

	port := rp.getPort()

	if err := rp.srvMgr.StartServer(rp.client, port); err != nil {
		rp.statusEntry.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	rp.statusEntry.SetText(fmt.Sprintf("Server running on port %d", port))
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn.Enable()
}

func (rp *RemotePanel) onStopServer() {
	if rp.client == nil {
		return
	}

	if err := rp.srvMgr.StopServer(rp.client); err != nil {
		rp.statusEntry.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	rp.statusEntry.SetText("Server stopped")
	rp.startSrvBtn.Enable()
	rp.stopSrvBtn.Disable()
}

func (rp *RemotePanel) onInstall() {
	if rp.client == nil {
		return
	}

	rp.installBtn.Disable()
	rp.statusEntry.SetText("Installing iperf3...")

	go func() {
		defer rp.installBtn.Enable()

		if err := rp.client.InstallIperf3(); err != nil {
			rp.statusEntry.SetText(fmt.Sprintf("Install failed: %v", err))
			return
		}

		rp.statusEntry.SetText("iperf3 installed successfully")
	}()
}

