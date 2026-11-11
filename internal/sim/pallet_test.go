package sim

import (
	"math"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// palletLine is a conveyor 4 m long, running on its own, with a light barrier across it at
// 3 m and a stop at its end, for pallets of 1200 by 800 mm and cardboard boxes.
func palletLine(t *testing.T) (*Plant, *model.Plan) {
	t.Helper()
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/line/station.json": {Data: []byte(`{"format":5,"id":"line","name":"Line","workpieces":"pallets",
			"modules":[{"id":"c","module":"conveyor"}],"plc":{}}`)},
		"modules/conveyor/module.json": {Data: []byte(`{"format":2,"id":"conveyor","version":1,"name":"Conveyor",
			"devices":[
				{"type":"logic","id":"run","expr":"1"},
				{"type":"sensor","id":"barrier","origin":[3000,-600,560],"direction":[0,1,0],"reach":1200,"detect":"beam","threshold":50},
				{"type":"solid","id":"stop","box":{"from":[3900,-450,500],"to":[3950,450,700]},"label":"The end stop"}],
			"transport":[{"id":"surface","drive":"run.on","speed":500,"path":{"line":{"from":[0,0,500],"to":[4000,0,500]}}}],
			"actions":[{"id":"put","type":"placeWorkpiece","transport":"surface","at":700}],
			"terminal":{}}`)},
		"workpieces/pallets.json": {Data: []byte(`{"format":2,"id":"pallets","diameter":0,"height":144,"points":[],
			"kinds":[
				{"id":"pallet","label":"Pallet","family":"pallet","size":[1200,800],"height":144,"material":{"type":"MeshLambertMaterial","color":"#c8a46e"}},
				{"id":"box","label":"Box","family":"box","size":[600,400],"height":300,"reflectivity":40,
					"onto":{"family":"pallet","free":true},"material":{"type":"MeshLambertMaterial","color":"#b07a45"}}]}`)},
	}, catalog.Base).Load("line")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(plan), plan
}

func TestPalletsRunAsBlocks(t *testing.T) {
	p, plan := palletLine(t)
	put := plan.Actions["c.put"]
	if err := p.place(&put, "pallet", false); err != nil {
		t.Fatal(err)
	}
	if h := p.headingOf(&p.parts[0]); h != (model.Point{X: 1}) {
		t.Fatalf("a pallet put on the belt points along it: %v", h)
	}
	barrier := plan.State("c.barrier.detected")
	// The barrier sees the pallet once its front, 600 mm ahead of its middle, reaches 3 m.
	for p.parts[0].s+600 < 2990 {
		if p.v[barrier] == 1 {
			t.Fatalf("seen at %v, before its front reached the barrier", p.parts[0].s)
		}
		p.Step(dt, plcio.Image{})
	}
	run(p, 0.1, plcio.Image{})
	if p.v[barrier] != 1 {
		t.Fatalf("not seen with its front at %v", p.parts[0].s+600)
	}
	// It runs up to the stop, its front at the stop's face, and stays there.
	run(p, 5, plcio.Image{})
	if s := p.parts[0].s; math.Abs(s+600-3900) > 1 {
		t.Fatalf("the pallet stops with its front at the stop: middle at %v", s)
	}
	if len(p.Collisions()) != 0 {
		t.Errorf("running into the stop is no collision: %v", p.Collisions())
	}
	// A second pallet queues right behind it, 1200 mm further back.
	if err := p.place(&put, "pallet", false); err != nil {
		t.Fatal(err)
	}
	run(p, 6, plcio.Image{})
	if gap := p.parts[0].s - p.parts[1].s; math.Abs(gap-1200) > 1 {
		t.Errorf("the second pallet stops against the first: %v apart", gap)
	}
	// There is no room for a third one where the second stands.
	at := put
	at.At = p.parts[1].s
	if err := p.place(&at, "pallet", false); err != ErrOccupied {
		t.Errorf("a pallet put onto another one: %v", err)
	}
}

func TestBoxesStackOnAPallet(t *testing.T) {
	p, plan := palletLine(t)
	put := plan.Actions["c.put"]
	if err := p.place(&put, "pallet", false); err != nil {
		t.Fatal(err)
	}
	pallet := p.world(&p.parts[0])
	deck := pallet.Z + 144
	// drop lets a box go with its base at a point, as a gripper's release does.
	drop := func(dx, dy, z float64) bool {
		c := model.Point{X: pallet.X + dx, Y: pallet.Y + dy, Z: z}
		p.parts = append(p.parts, part{id: 100 + len(p.parts), kind: "box", fall: &fall{at: c}, dir: model.Point{X: 1}})
		ok := p.joinOnto(len(p.parts)-1, c, 5)
		p.sweep()
		return ok
	}
	if !drop(300, 200, deck+2) || !drop(-300, -200, deck) {
		t.Fatal("boxes set onto the deck join the pallet")
	}
	if !drop(300, 200, deck+300+1) {
		t.Fatal("a box set onto a box joins the pallet on top of it")
	}
	if drop(300, 200, deck+300+100) {
		t.Error("a box let go high above lands on nothing it joins")
	}
	if drop(900, 0, deck) {
		t.Error("a box beside the pallet does not join it")
	}
	loads := p.parts[0].loads
	if len(loads) != 3 || loads[0].off != (model.Point{X: 300, Y: 200, Z: 144}) || loads[1].off != (model.Point{X: -300, Y: -200, Z: 144}) || loads[2].off != (model.Point{X: 300, Y: 200, Z: 444}) {
		t.Fatalf("loads %+v", loads)
	}
	if h := p.heightOf(&p.parts[0]); h != 744 {
		t.Errorf("the pallet with two layers is 744 mm high, not %v", h)
	}
	// The loaded pallet runs to the stop and takes its boxes along.
	run(p, 8, plcio.Image{})
	v := p.Workpieces()
	if len(v) != 1 || len(v[0].Loads) != 3 || v[0].Loads[2].At.Z != 444 || math.Abs(v[0].At.X+600-3900) > 1 {
		t.Errorf("the view: %+v", v)
	}
}
