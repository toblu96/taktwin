package sim

import (
	"time"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/st"
)

// program is a module's own controller in Structured Text, running on the simulation's clock.
type program struct {
	plan    *model.ProgramPlan
	rt      *st.Runtime
	in, out []handle
	clock   float64 // seconds since the reset
	next    float64 // when it runs next
	failed  bool
}

// handle joins a variable of a program to a state.
type handle struct {
	h     st.Handle
	state int
}

// startPrograms starts the program devices from their initial values.
func (p *Plant) startPrograms() {
	p.programs = p.programs[:0]
	for i := range p.plan.Programs {
		pl := &p.plan.Programs[i]
		pr := program{plan: pl, rt: st.NewRuntime(pl.Program)}
		ok := pr.rt.Start() == nil
		for _, list := range []struct {
			from []model.VarBinding
			to   *[]handle
		}{{pl.In, &pr.in}, {pl.Out, &pr.out}} {
			for _, b := range list.from {
				h, err := pr.rt.Handle(b.Path)
				if err != nil {
					ok = false
					p.programFails(&pr, err.Error())
					continue
				}
				*list.to = append(*list.to, handle{h, b.State})
			}
		}
		pr.failed = !ok
		p.programs = append(p.programs, pr)
	}
}

// updatePrograms runs each program device whose cycle has come: it takes the states it reads,
// runs, and sets the states it writes, which keep their values until it runs again.
func (p *Plant) updatePrograms(dt float64) {
	for i := range p.programs {
		pr := &p.programs[i]
		pr.clock += dt
		if pr.failed || pr.clock+1e-9 < pr.next {
			continue
		}
		pr.next = pr.clock + pr.plan.Cycle
		for _, h := range pr.in {
			h.h.Set(p.v[h.state])
		}
		if err := pr.rt.Run(time.Duration(pr.clock * float64(time.Second))); err != nil {
			pr.failed = true
			p.programFails(pr, err.Error())
			continue
		}
		for _, h := range pr.out {
			p.v[h.state] = h.h.Get()
		}
	}
}

// programFails reports a program that stopped, as the top bar does a collision.
func (p *Plant) programFails(pr *program, why string) {
	text := "The controller " + pr.plan.Name + " stopped: " + why
	p.hit = append(p.hit, text)
	p.everHit = append(p.everHit, text)
}

// updateCounts counts the workpieces on each transport.
func (p *Plant) updateCounts() {
	for i := range p.segments {
		s := &p.segments[i]
		p.v[s.Count] = 0
		p.v[s.Before] = b2f(s.Prev >= 0)
	}
	for i := range p.parts {
		if w := &p.parts[i]; w.fall == nil && !w.gone {
			p.v[p.segments[w.seg].Count]++
		}
	}
}
