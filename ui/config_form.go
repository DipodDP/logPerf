package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"iperf-tool/internal/iperf"
)

// ConfigForm holds the GUI form fields for iperf3 configuration.
type ConfigForm struct {
	serverEntry   *widget.Entry
	portEntry     *widget.Entry
	parallelEntry *widget.Select
	intervalEntry *widget.Entry
	durationEntry *widget.Entry
	protocolRadio *widget.RadioGroup
	binaryEntry   *widget.Entry
	form          *fyne.Container
}

// NewConfigForm creates a new configuration form with default values.
func NewConfigForm() *ConfigForm {
	cf := &ConfigForm{}

	cf.serverEntry = widget.NewEntry()
	cf.serverEntry.SetPlaceHolder("e.g. 192.168.1.1")

	cf.portEntry = widget.NewEntry()
	cf.portEntry.SetText("5201")

	parallelOpts := make([]string, 16)
	for i := range parallelOpts {
		parallelOpts[i] = strconv.Itoa(i + 1)
	}
	cf.parallelEntry = widget.NewSelect(parallelOpts, nil)
	cf.parallelEntry.SetSelected("1")

	cf.intervalEntry = widget.NewEntry()
	cf.intervalEntry.SetText("1")

	cf.durationEntry = widget.NewEntry()
	cf.durationEntry.SetText("10")

	cf.protocolRadio = widget.NewRadioGroup([]string{"TCP", "UDP"}, nil)
	cf.protocolRadio.SetSelected("TCP")
	cf.protocolRadio.Horizontal = true

	cf.binaryEntry = widget.NewEntry()
	cf.binaryEntry.SetText("iperf3")
	cf.binaryEntry.SetPlaceHolder("path to iperf3 binary")

	cf.form = container.NewVBox(
		widget.NewLabel("Server Address"),
		cf.serverEntry,
		widget.NewLabel("Port"),
		cf.portEntry,
		widget.NewLabel("Parallel Streams"),
		cf.parallelEntry,
		widget.NewLabel("Interval (sec)"),
		cf.intervalEntry,
		widget.NewLabel("Duration (sec)"),
		cf.durationEntry,
		widget.NewLabel("Protocol"),
		cf.protocolRadio,
		widget.NewLabel("iperf3 Binary"),
		cf.binaryEntry,
	)

	return cf
}

// Container returns the form's Fyne container.
func (cf *ConfigForm) Container() *fyne.Container {
	return cf.form
}

// LoadPreferences restores form values from persistent preferences.
func (cf *ConfigForm) LoadPreferences(prefs fyne.Preferences) {
	if v := prefs.String("config.server_addr"); v != "" {
		cf.serverEntry.SetText(v)
	}
	if v := prefs.String("config.port"); v != "" {
		cf.portEntry.SetText(v)
	}
	if v := prefs.String("config.parallel"); v != "" {
		cf.parallelEntry.SetSelected(v)
	}
	if v := prefs.String("config.interval"); v != "" {
		cf.intervalEntry.SetText(v)
	}
	if v := prefs.String("config.duration"); v != "" {
		cf.durationEntry.SetText(v)
	}
	if v := prefs.String("config.protocol"); v != "" {
		cf.protocolRadio.SetSelected(v)
	}
	if v := prefs.String("config.binary"); v != "" {
		cf.binaryEntry.SetText(v)
	}
}

// SavePreferences persists form values to preferences.
func (cf *ConfigForm) SavePreferences(prefs fyne.Preferences) {
	prefs.SetString("config.server_addr", cf.serverEntry.Text)
	prefs.SetString("config.port", cf.portEntry.Text)
	prefs.SetString("config.parallel", cf.parallelEntry.Selected)
	prefs.SetString("config.interval", cf.intervalEntry.Text)
	prefs.SetString("config.duration", cf.durationEntry.Text)
	prefs.SetString("config.protocol", cf.protocolRadio.Selected)
	prefs.SetString("config.binary", cf.binaryEntry.Text)
}

// Config builds an IperfConfig from the current form values.
func (cf *ConfigForm) Config() iperf.IperfConfig {
	port, _ := strconv.Atoi(cf.portEntry.Text)
	parallel, _ := strconv.Atoi(cf.parallelEntry.Selected)
	interval, _ := strconv.Atoi(cf.intervalEntry.Text)
	duration, _ := strconv.Atoi(cf.durationEntry.Text)

	protocol := "tcp"
	if cf.protocolRadio.Selected == "UDP" {
		protocol = "udp"
	}

	return iperf.IperfConfig{
		BinaryPath: cf.binaryEntry.Text,
		ServerAddr: cf.serverEntry.Text,
		Port:       port,
		Parallel:   parallel,
		Duration:   duration,
		Interval:   interval,
		Protocol:   protocol,
	}
}
