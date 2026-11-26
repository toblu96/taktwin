package model

import (
	"testing"
)

// Each signal of the reference station leads to the module and device it belongs to, and that
// device says where it is: a sensor by its head, a cylinder and its reed switches by the joint
// the cylinder moves, a button by what it presses.
func TestASignalLeadsToItsDevice(t *testing.T) {
	plan := referencePlan(t, nil)
	instances := map[string]SceneInstance{}
	for _, in := range plan.Scene.Instances {
		instances[in.ID] = in
	}
	owners := map[string][2]string{}
	for _, in := range plan.Scene.Instances {
		for _, c := range in.Connectors {
			for _, p := range c.Pins {
				if p.Address != "" && p.Owner != "" {
					owners[p.Address] = [2]string{p.Owner, p.Device}
				}
			}
		}
	}
	found := map[string]bool{}
	for _, s := range append(plan.Scene.PLC.Inputs, plan.Scene.PLC.Outputs...) {
		o, ok := owners[s.Address]
		if !ok {
			continue
		}
		in, ok := instances[o[0]]
		if !ok {
			t.Errorf("%s belongs to %s, which is no module of the station", s.Name, o[0])
			continue
		}
		for _, d := range in.Devices {
			if d.ID != o[1] {
				continue
			}
			switch {
			case d.Spot != nil:
				found["spot"] = true
			case d.Joint >= 0:
				found["joint"] = true
			case d.Press != "":
				found["press"] = true
			case d.Node != "":
				found["node"] = true
			}
		}
	}
	for _, kind := range []string{"spot", "joint", "press"} {
		if !found[kind] {
			t.Errorf("no signal leads to a device found by its %s: %v", kind, found)
		}
	}
	if len(owners) < 20 {
		t.Errorf("only %d signals lead to a module", len(owners))
	}
}
