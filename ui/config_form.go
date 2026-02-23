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
	serverEntry      *widget.Entry
	portEntry        *widget.Entry
	parallelEntry    *widget.Select
	intervalEntry    *widget.Entry
	durationEntry    *widget.Entry
	protocolRadio    *widget.RadioGroup
	directionRadio   *widget.RadioGroup
	blockSizeEntry   *widget.Entry
	bandwidthEntry   *widget.Entry
	congestionSelect *widget.Select
	measurePingCheck *widget.Check
	binaryEntry      *widget.Entry
	form             *fyne.Container
}

// NewConfigForm creates a new configuration form with default values.
func NewConfigForm() *ConfigForm {
	cf := &ConfigForm{}

	cf.serverEntry = widget.NewEntry()
	cf.serverEntry.SetPlaceHolder("192.168.1.1")

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

	cf.directionRadio = widget.NewRadioGroup([]string{"Normal", "Reverse", "Bidir"}, nil)
	cf.directionRadio.SetSelected("Normal")
	cf.directionRadio.Horizontal = true

	cf.blockSizeEntry = widget.NewEntry()
	cf.blockSizeEntry.SetPlaceHolder("default")

	cf.bandwidthEntry = widget.NewEntry()
	cf.bandwidthEntry.SetPlaceHolder("100M, 1G")

	cf.congestionSelect = widget.NewSelect([]string{"default", "bbr", "cubic", "reno", "vegas"}, nil)
	cf.congestionSelect.SetSelected("default")

	cf.measurePingCheck = widget.NewCheck("Measure Ping", nil)

	cf.binaryEntry = widget.NewEntry()
	cf.binaryEntry.SetText("iperf3")
	cf.binaryEntry.SetPlaceHolder("/usr/local/bin/iperf3")

	connection := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Server", cf.serverEntry),
			widget.NewFormItem("Port", cf.portEntry),
			widget.NewFormItem("Protocol", cf.protocolRadio),
		),
	)

	testParams := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Duration", cf.durationEntry),
			widget.NewFormItem("Interval", cf.intervalEntry),
			widget.NewFormItem("Direction", cf.directionRadio),
		),
		cf.measurePingCheck,
	)

	performance := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Streams", cf.parallelEntry),
			widget.NewFormItem("Bandwidth", cf.bandwidthEntry),
			widget.NewFormItem("Block Size", cf.blockSizeEntry),
			widget.NewFormItem("Congestion", cf.congestionSelect),
			widget.NewFormItem("iperf3 path", cf.binaryEntry),
		),
	)

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Connection", connection),
		widget.NewAccordionItem("Test Parameters", testParams),
		widget.NewAccordionItem("Performance", performance),
	)

	// Open the "Connection" section by default as it contains essential fields.
	accordion.Open(0)

	cf.form = container.NewVBox(accordion)

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
	if v := prefs.String("config.direction"); v != "" {
		cf.directionRadio.SetSelected(v)
	}
	if v := prefs.String("config.block_size"); v != "" {
		cf.blockSizeEntry.SetText(v)
	}
	if v := prefs.String("config.bandwidth"); v != "" {
		cf.bandwidthEntry.SetText(v)
	}
	if v := prefs.String("config.congestion"); v != "" {
		cf.congestionSelect.SetSelected(v)
	}
	cf.measurePingCheck.SetChecked(prefs.Bool("config.measure_ping"))
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
	prefs.SetString("config.direction", cf.directionRadio.Selected)
	prefs.SetString("config.block_size", cf.blockSizeEntry.Text)
	prefs.SetString("config.bandwidth", cf.bandwidthEntry.Text)
	prefs.SetString("config.congestion", cf.congestionSelect.Selected)
	prefs.SetBool("config.measure_ping", cf.measurePingCheck.Checked)
	prefs.SetString("config.binary", cf.binaryEntry.Text)
}

// Config builds an IperfConfig from the current form values.
// Uses safe parsing with default values for any invalid inputs.
func (cf *ConfigForm) Config() iperf.IperfConfig {
	port := parsePort(cf.portEntry.Text, 5201)
	parallel := parseIntOrDefault(cf.parallelEntry.Selected, 1)
	interval := parseIntOrDefault(cf.intervalEntry.Text, 1)
	duration := parseIntOrDefault(cf.durationEntry.Text, 10)
	blockSize := parseIntOrDefault(cf.blockSizeEntry.Text, 0)

	protocol := "tcp"
	if cf.protocolRadio.Selected == "UDP" {
		protocol = "udp"
	}

	reverse := cf.directionRadio.Selected == "Reverse"
	bidir := cf.directionRadio.Selected == "Bidir"

	congestion := ""
	if cf.congestionSelect.Selected != "default" {
		congestion = cf.congestionSelect.Selected
	}

	return iperf.IperfConfig{
		BinaryPath:  cf.binaryEntry.Text,
		ServerAddr:  cf.serverEntry.Text,
		Port:        port,
		Parallel:    parallel,
		Duration:    duration,
		Interval:    interval,
		Protocol:    protocol,
		BlockSize:   blockSize,
		Reverse:     reverse,
		Bidir:       bidir,
		Bandwidth:   cf.bandwidthEntry.Text,
		Congestion:  congestion,
		MeasurePing: cf.measurePingCheck.Checked,
	}
}
