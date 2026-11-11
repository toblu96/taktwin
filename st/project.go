package st

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Project is what a PLC runs: its source files, and the task that runs its programs.
type Project struct {
	// Files are the sources by name, such as "PLC_PRG.st".
	Files map[string]string
	// Programs are the programs the task calls each cycle, in order; empty is PLC_PRG.
	Programs []string
	// Cycle is the task's interval; 0 is 10 ms.
	Cycle time.Duration
}

// Severity says whether a message stops a program from being built.
type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

// Diagnostic is a message about the source, with where it is.
type Diagnostic struct {
	Pos      Pos      `json:"pos"`
	Severity Severity `json:"severity"`
	Msg      string   `json:"msg"`
}

func (d Diagnostic) String() string { return fmt.Sprintf("%s: %s: %s", d.Pos, d.Severity, d.Msg) }

// Program is a compiled project, ready to run.
type Program struct {
	pous     map[string]*POU // by upper-case name
	order    []*POU          // in the order they were declared
	types    map[string]*Type
	globals  []*Var
	global   map[string]*Var            // by upper-case name
	lists    map[string]map[string]*Var // global variable lists by file name, then variable
	programs []*POU                     // the task's programs, in order
	cycle    time.Duration
	inputs   []ioBit
	outputs  []ioBit
}

// ioBit is a variable at a direct address: a bit of the input or output image.
type ioBit struct {
	Byte, Bit int
	global    *Var // a global, or
	pou       *POU // a variable of a program instance
	slot      int
}

// Cycle is the interval of the task that runs the program.
func (p *Program) Cycle() time.Duration { return p.cycle }

// Compile builds a project. It gives a program only when there are no errors; warnings
// come either way.
func Compile(proj Project) (*Program, []Diagnostic) {
	c := newCompiler()
	names := make([]string, 0, len(proj.Files))
	for name := range proj.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	var files []*File
	for _, name := range names {
		f, err := Parse(name, proj.Files[name])
		if err != nil {
			c.errAt(errPos(err, name), "%s", errMsg(err))
			continue
		}
		files = append(files, f)
	}
	// While a file does not parse, what it declares is unknown to the others: report only
	// that, rather than everything that follows from it.
	if len(c.diags) > 0 {
		return nil, c.diags
	}
	parsed := true
	c.declare(files)
	c.prog.cycle = proj.Cycle
	if c.prog.cycle <= 0 {
		c.prog.cycle = 10 * time.Millisecond
	}
	programs := proj.Programs
	if len(programs) == 0 {
		programs = []string{"PLC_PRG"}
	}
	for _, name := range programs {
		p := c.prog.pous[strings.ToUpper(name)]
		switch {
		case p == nil && parsed:
			c.errAt(Pos{File: "task"}, "the task calls %s, but there is no program of that name", name)
		case p == nil:
		case p.Kind != KindProgram:
			c.errAt(p.Decl.Pos, "the task calls %s, which is a %s, not a PROGRAM", name, p.Kind)
		default:
			c.prog.programs = append(c.prog.programs, p)
		}
	}
	c.compileBodies()
	sort.SliceStable(c.diags, func(i, j int) bool {
		a, b := c.diags[i].Pos, c.diags[j].Pos
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	for _, d := range c.diags {
		if d.Severity == SevError {
			return nil, c.diags
		}
	}
	return c.prog, c.diags
}

func errPos(err error, file string) Pos {
	switch e := err.(type) {
	case *LexError:
		return e.Pos
	case *SyntaxError:
		return e.Pos
	}
	return Pos{File: file, Line: 1, Col: 1}
}

func errMsg(err error) string {
	switch e := err.(type) {
	case *LexError:
		return e.Msg
	case *SyntaxError:
		return e.Msg
	}
	return err.Error()
}

// listName is the name a file gives the global variables it declares, as a GVL is named.
func listName(file string) string {
	return strings.TrimSuffix(path.Base(file), path.Ext(file))
}
