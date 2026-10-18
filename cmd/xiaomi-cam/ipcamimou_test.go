package main

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// The Imou cloud settings of a saved camera come back in the edit form, and the form
// copies them to the camera.
func TestIPFormImou(t *testing.T) {
	c := &ipCamera{Kind: "rtsp", Main: "rtsp://admin:code@192.168.3.177/cam/realmonitor?channel=1&subtype=0",
		ImouApp: "lcAPP", ImouSecret: "SECRET", ImouDevice: "C41D4AEPCG84966"}
	f := ipFormOf(c)
	if f.ImouApp != "lcAPP" || f.ImouSecret != "SECRET" || f.ImouDevice != "C41D4AEPCG84966" {
		t.Fatalf("form %+v", f)
	}
	var nc ipCamera
	f.ImouDC = "fk"
	f.imouTo(&nc)
	if nc.ImouApp != "lcAPP" || nc.ImouDevice != "C41D4AEPCG84966" || nc.ImouDC != "" {
		t.Errorf("camera %+v", nc)
	}

	test.NewTempApp(t)
	w := test.NewWindow(widget.NewLabel(""))
	defer w.Close()
	w.Resize(fyne.NewSize(900, 900))
	showAddIPCamera(w, f, "test", func(string, *ipCamera) error { return nil })
	var texts []string
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *widget.Entry:
			texts = append(texts, x.Text)
		case *ptrSelect:
			texts = append(texts, x.Selected)
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(x).Objects() {
				walk(c)
			}
		case *fyne.Container:
			for _, c := range x.Objects {
				walk(c)
			}
		}
	}
	for _, o := range w.Canvas().Overlays().List() {
		walk(o)
	}
	for _, want := range []string{"lcAPP", "SECRET", T("zapisana") + " – Imou (C41D4AEPCG84966)"} {
		if !slices.Contains(texts, want) {
			t.Errorf("the form has no %q (fields %q)", want, texts)
		}
	}
}
