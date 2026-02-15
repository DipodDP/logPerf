package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// StyledButton is a button with custom background and text colors.
type StyledButton struct {
	widget.Button
	bgColor     color.Color
	txtColor    color.Color
	origBg      color.Color
	origTxt     color.Color
}

// NewStyledButton creates a button with custom colors.
func NewStyledButton(label string, tapped func(), bgColor, txtColor color.Color) *StyledButton {
	btn := &StyledButton{
		bgColor:  bgColor,
		txtColor: txtColor,
		origBg:   bgColor,
		origTxt:  txtColor,
	}
	btn.Text = label
	btn.OnTapped = tapped
	btn.ExtendBaseWidget(btn)
	return btn
}

// Enable restores original colors.
func (b *StyledButton) Enable() {
	b.Button.Enable()
	b.bgColor = b.origBg
	b.txtColor = b.origTxt
	b.Refresh()
}

// Disable dims the button.
func (b *StyledButton) Disable() {
	b.Button.Disable()
	b.Refresh()
}

// CreateRenderer returns a custom renderer.
func (b *StyledButton) CreateRenderer() fyne.WidgetRenderer {
	b.ExtendBaseWidget(b)

	bg := canvas.NewRectangle(b.bgColor)
	bg.CornerRadius = theme.InputRadiusSize()

	label := canvas.NewText(b.Text, b.txtColor)
	label.Alignment = fyne.TextAlignCenter
	label.TextStyle = fyne.TextStyle{Bold: true}

	return &styledBtnRenderer{
		btn:    b,
		bg:     bg,
		label:  label,
		objects: []fyne.CanvasObject{bg, label},
	}
}

type styledBtnRenderer struct {
	btn     *StyledButton
	bg      *canvas.Rectangle
	label   *canvas.Text
	objects []fyne.CanvasObject
}

func (r *styledBtnRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	labelMin := r.label.MinSize()
	r.label.Move(fyne.NewPos(
		(size.Width-labelMin.Width)/2,
		(size.Height-labelMin.Height)/2,
	))
	r.label.Resize(labelMin)
}

func (r *styledBtnRenderer) MinSize() fyne.Size {
	labelMin := r.label.MinSize()
	pad := theme.InnerPadding()
	return fyne.NewSize(labelMin.Width+pad*4, labelMin.Height+pad*2)
}

func (r *styledBtnRenderer) Refresh() {
	r.label.Text = r.btn.Text

	if r.btn.Disabled() {
		r.bg.FillColor = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
		r.label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	} else {
		r.bg.FillColor = r.btn.bgColor
		r.label.Color = r.btn.txtColor
	}

	r.bg.Refresh()
	r.label.Refresh()
}

func (r *styledBtnRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *styledBtnRenderer) Destroy()                     {}
