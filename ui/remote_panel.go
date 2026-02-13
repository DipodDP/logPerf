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
	startSrvBtn   *widget.Button
	stopSrvBtn    *widget.Button
	statusLabel   *widget.Label

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
	rp.hostEntry.SetPlaceHolder("SSH host")

	rp.userEntry = widget.NewEntry()
	rp.userEntry.SetPlaceHolder("username")

	rp.keyPathEntry = widget.NewEntry()
	rp.keyPathEntry.SetPlaceHolder("~/.ssh/id_rsa")

	rp.passwordEntry = widget.NewPasswordEntry()
	rp.passwordEntry.SetPlaceHolder("password (optional)")

	rp.portEntry = widget.NewEntry()
	rp.portEntry.SetText("5201")

	rp.statusLabel = widget.NewLabel("Disconnected")

	rp.connectBtn = widget.NewButton("Connect", rp.onConnect)
	rp.disconnectBtn = widget.NewButton("Disconnect", rp.onDisconnect)
	rp.disconnectBtn.Disable()

	rp.startSrvBtn = widget.NewButton("Start Server", rp.onStartServer)
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn = widget.NewButton("Stop Server", rp.onStopServer)
	rp.stopSrvBtn.Disable()

	rp.container = container.NewVBox(
		widget.NewLabelWithStyle("Remote Server (SSH)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Host"),
		rp.hostEntry,
		widget.NewLabel("User"),
		rp.userEntry,
		widget.NewLabel("SSH Key Path"),
		rp.keyPathEntry,
		widget.NewLabel("Password"),
		rp.passwordEntry,
		container.NewHBox(rp.connectBtn, rp.disconnectBtn),
		widget.NewSeparator(),
		widget.NewLabel("iperf3 Server Port"),
		rp.portEntry,
		container.NewHBox(rp.startSrvBtn, rp.stopSrvBtn),
		rp.statusLabel,
	)

	return rp
}

// Container returns the panel's container.
func (rp *RemotePanel) Container() *fyne.Container {
	return rp.container
}

func (rp *RemotePanel) onConnect() {
	cfg := internalssh.ConnectConfig{
		Host:     rp.hostEntry.Text,
		Port:     22,
		User:     rp.userEntry.Text,
		KeyPath:  rp.keyPathEntry.Text,
		Password: rp.passwordEntry.Text,
	}

	client, err := internalssh.Connect(cfg)
	if err != nil {
		rp.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	rp.client = client
	rp.statusLabel.SetText(fmt.Sprintf("Connected to %s", cfg.Host))
	rp.connectBtn.Disable()
	rp.disconnectBtn.Enable()
	rp.startSrvBtn.Enable()
}

func (rp *RemotePanel) onDisconnect() {
	if rp.client != nil {
		rp.client.Close()
		rp.client = nil
	}
	rp.statusLabel.SetText("Disconnected")
	rp.connectBtn.Enable()
	rp.disconnectBtn.Disable()
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn.Disable()
}

func (rp *RemotePanel) onStartServer() {
	if rp.client == nil {
		return
	}

	port := 5201
	if v := rp.portEntry.Text; v != "" {
		fmt.Sscanf(v, "%d", &port)
	}

	if err := rp.srvMgr.StartServer(rp.client, port); err != nil {
		rp.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	rp.statusLabel.SetText(fmt.Sprintf("Server running on port %d", port))
	rp.startSrvBtn.Disable()
	rp.stopSrvBtn.Enable()
}

func (rp *RemotePanel) onStopServer() {
	if rp.client == nil {
		return
	}

	if err := rp.srvMgr.StopServer(rp.client); err != nil {
		rp.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	rp.statusLabel.SetText("Server stopped")
	rp.startSrvBtn.Enable()
	rp.stopSrvBtn.Disable()
}

