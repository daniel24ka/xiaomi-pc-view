package xccore

import "testing"

func TestModelSpecs(t *testing.T) {
	c200 := SpecForModel("chuangmi.camera.046c04")
	cw700s := SpecForModel("isa.camera.700sa")
	if c200 == nil || cw700s == nil || SpecForModel("chuangmi.camera.unknown") != nil {
		t.Fatal("spec table")
	}

	// the 700sa family: ids and values as they are
	if p, ok := cw700s.mapProp(PropSiren); !ok || !p.Same(PropSiren) {
		t.Error("700sa siren")
	}
	if v, ok := cw700s.toModel(PropNightShot, 5); !ok || v != 5 {
		t.Error("700sa night value")
	}

	// C200: no spotlight, siren, zoom or points; night-shot values translated
	for _, p := range []MiotProp{PropLight, PropBrightness, PropSiren, PropZoom, PropMotorLocation, PropPresets} {
		if _, ok := c200.mapProp(p); ok {
			t.Errorf("C200 has no %d.%d", p.Siid, p.Piid)
		}
	}
	if _, ok := c200.mapProp(PropSDStatus); !ok {
		t.Error("C200 SD status")
	}
	for canon, model := range map[int]int{2: 2, 1: 0, 0: 1} {
		if v, ok := c200.toModel(PropNightShot, canon); !ok || v != model {
			t.Errorf("C200 night %d -> %v", canon, v)
		}
		if v := c200.fromModel(PropNightShot, float64(model)); v != float64(canon) {
			t.Errorf("C200 night back %d -> %v", model, v)
		}
	}
	for _, v := range []int{3, 4, 5} { // full color modes need the spotlight
		if c200.hasValue(PropNightShot, v) {
			t.Errorf("C200 night value %d", v)
		}
	}
	if !c200.PanTilt || cw700s.PanTilt {
		t.Error("pan/tilt")
	}

	// no spec: nothing is mapped
	var none *ModelSpec
	if _, ok := none.mapProp(PropNightShot); ok || none.hasValue(PropNightShot, 2) {
		t.Error("nil spec")
	}
}
