// Package plcprog keeps the Structured Text projects of a station's PLCs and runs them as
// soft PLCs inside the app, which the simulation reaches over OPC UA as it would CODESYS.
// The language itself is the st module; this package only stores, builds and hosts.
package plcprog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/st"
)

// BitsFile is the global variable list the app writes from the station's I/O table: the
// Bits list, as a CODESYS project offers it over OPC UA, with every signal at its address.
const BitsFile = "Bits.st"

// metaFile holds the task of a project.
const metaFile = "project.json"

// Signal is a bit of the station's I/O table, as the PLC reaches it.
type Signal struct {
	Name        string // such as B1
	Address     string // such as IB0Bit00, as the PLC module names it
	Variable    string // the name a program uses, such as band, if the station gives one
	Description string
	Input       bool
	Byte, Bit   int
}

// Meta is the task of a project: the programs it calls each cycle, its interval, and the
// variable that holds the step of the sequence, to show in the simulation.
type Meta struct {
	Programs []string `json:"programs"`
	Cycle    string   `json:"cycle"`
	Step     string   `json:"step,omitempty"`
}

// Project is a PLC's project in its folder.
type Project struct {
	Dir  string
	Meta Meta
}

// File is a source file of a project.
type File struct {
	Name     string `json:"name"`
	ReadOnly bool   `json:"readOnly"`
	// Kind is PROGRAM, FUNCTION_BLOCK, FUNCTION, GVL or TYPE, from what the file declares.
	Kind string `json:"kind"`
}

var fileName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.st$`)

// Open reads the project in a folder, making it from the template first if there is none.
func Open(dir string, signals []Signal) (*Project, error) {
	if _, err := os.Stat(filepath.Join(dir, metaFile)); errors.Is(err, os.ErrNotExist) {
		if err := create(dir, signals); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return nil, err
	}
	p := &Project{Dir: dir}
	if err := json.Unmarshal(data, &p.Meta); err != nil {
		return nil, fmt.Errorf("%s: %w", metaFile, err)
	}
	if err := os.WriteFile(filepath.Join(dir, BitsFile), []byte(bitsList(signals)), 0o644); err != nil {
		return nil, err
	}
	return p, nil
}

// Files lists the project's source files, the Bits list first.
func (p *Project) Files() ([]File, error) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return nil, err
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".st") {
			continue
		}
		src, _ := os.ReadFile(filepath.Join(p.Dir, e.Name()))
		out = append(out, File{Name: e.Name(), ReadOnly: e.Name() == BitsFile, Kind: kindOf(string(src))})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReadOnly != out[j].ReadOnly {
			return out[i].ReadOnly
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

var declares = regexp.MustCompile(`(?i)\b(PROGRAM|FUNCTION_BLOCK|FUNCTION|VAR_GLOBAL|TYPE)\b`)

// kindOf tells what a file declares first.
func kindOf(src string) string {
	// Comments do not count.
	src = regexp.MustCompile(`(?s)\(\*.*?\*\)|//[^\n]*`).ReplaceAllString(src, "")
	m := declares.FindString(src)
	switch strings.ToUpper(m) {
	case "VAR_GLOBAL":
		return "GVL"
	case "":
		return "EMPTY"
	}
	return strings.ToUpper(m)
}

// Read gives a file's source.
func (p *Project) Read(name string) (string, error) {
	if !fileName.MatchString(name) {
		return "", fmt.Errorf("%q is no source file of the project", name)
	}
	data, err := os.ReadFile(filepath.Join(p.Dir, name))
	return string(data), err
}

// Write saves a file's source. The Bits list is the station's and cannot be written.
func (p *Project) Write(name, src string) error {
	if err := p.writable(name); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.Dir, name), []byte(src), 0o644)
}

func (p *Project) writable(name string) error {
	if !fileName.MatchString(name) {
		return fmt.Errorf("%q is no name for a source file: letters, digits and _, ending in .st", name)
	}
	if strings.EqualFold(name, BitsFile) || strings.EqualFold(name, metaFile) {
		return fmt.Errorf("%s comes from the station's I/O table and is written anew on each build", name)
	}
	return nil
}

// Add makes a new file declaring a POU, a global variable list or a type, named after it.
func (p *Project) Add(name, kind string) (string, error) {
	file := name + ".st"
	if err := p.writable(file); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(p.Dir, file)); err == nil {
		return "", fmt.Errorf("%s exists already", file)
	}
	var src string
	switch strings.ToUpper(kind) {
	case "PROGRAM":
		src = fmt.Sprintf("PROGRAM %s\nVAR\nEND_VAR\n\nEND_PROGRAM\n", name)
	case "FUNCTION_BLOCK":
		src = fmt.Sprintf("FUNCTION_BLOCK %s\nVAR_INPUT\nEND_VAR\nVAR_OUTPUT\nEND_VAR\nVAR\nEND_VAR\n\nEND_FUNCTION_BLOCK\n", name)
	case "FUNCTION":
		src = fmt.Sprintf("FUNCTION %s : BOOL\nVAR_INPUT\nEND_VAR\n\n%s := FALSE;\nEND_FUNCTION\n", name, name)
	case "GVL":
		src = "VAR_GLOBAL\nEND_VAR\n"
	case "TYPE":
		src = fmt.Sprintf("TYPE %s :\nSTRUCT\nEND_STRUCT\nEND_TYPE\n", name)
	default:
		return "", fmt.Errorf("%q is no kind of file: PROGRAM, FUNCTION_BLOCK, FUNCTION, GVL or TYPE", kind)
	}
	return file, os.WriteFile(filepath.Join(p.Dir, file), []byte(src), 0o644)
}

// Remove deletes a file.
func (p *Project) Remove(name string) error {
	if err := p.writable(name); err != nil {
		return err
	}
	return os.Remove(filepath.Join(p.Dir, name))
}

// Rename gives a file another name.
func (p *Project) Rename(name, to string) error {
	if !strings.HasSuffix(to, ".st") {
		to += ".st"
	}
	if err := p.writable(name); err != nil {
		return err
	}
	if err := p.writable(to); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(p.Dir, to)); err == nil {
		return fmt.Errorf("%s exists already", to)
	}
	return os.Rename(filepath.Join(p.Dir, name), filepath.Join(p.Dir, to))
}

// SetMeta saves the task.
func (p *Project) SetMeta(m Meta) error {
	if _, err := parseCycle(m.Cycle); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.Dir, metaFile), data, 0o644); err != nil {
		return err
	}
	p.Meta = m
	return nil
}

func parseCycle(s string) (time.Duration, error) {
	d, err := st.ParseTime(s)
	if err != nil || d < time.Millisecond || d > time.Second {
		return 0, fmt.Errorf("the task's interval %q is no time from T#1ms to T#1s", s)
	}
	return d, nil
}

// Build compiles the project as it is in its folder.
func (p *Project) Build() (*st.Program, []st.Diagnostic, error) {
	cycle, err := parseCycle(p.Meta.Cycle)
	if err != nil {
		return nil, nil, err
	}
	files, err := p.Files()
	if err != nil {
		return nil, nil, err
	}
	sources := map[string]string{}
	for _, f := range files {
		src, err := p.Read(f.Name)
		if err != nil {
			return nil, nil, err
		}
		sources[f.Name] = src
	}
	prog, diags := st.Compile(st.Project{Files: sources, Programs: p.Meta.Programs, Cycle: cycle})
	return prog, diags, nil
}

// identifier makes a name a program can use from a signal's name, such as K1_2 for
// K1-2.
func identifier(s string) string {
	b := []rune(s)
	for i, r := range b {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			b[i] = '_'
		}
	}
	out := string(b)
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

func comment(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "(*", "( *"), "*)", "* )")
}

// bitsList writes the Bits list: every signal of the station at its address.
func bitsList(signals []Signal) string {
	var b strings.Builder
	b.WriteString("(* The Bits list: the station's I/O table, at the addresses of the PLC's image.\n   Taktwin writes it anew from the station on each build; change the names in IO_Mapping. *)\nVAR_GLOBAL\n")
	for _, s := range signals {
		dir := "Q"
		if s.Input {
			dir = "I"
		}
		fmt.Fprintf(&b, "\t%s AT %%%sX%d.%d : BOOL; (* %s: %s *)\n", s.Address, dir, s.Byte, s.Bit, comment(s.Name), comment(s.Description))
	}
	b.WriteString("END_VAR\n")
	return b.String()
}

// create makes a new project from the template: the step chain in
// a program of its own, a step handler that moves on to the next step after the step's
// delay, the mapping of the Bits list to names, and PLC_PRG calling them in that order.
func create(dir string, signals []Signal) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var names []string
	named := map[string]string{} // address to name
	for _, s := range signals {
		n := identifier(cmpOr(s.Variable, s.Name))
		if slices.ContainsFunc(names, func(o string) bool { return strings.EqualFold(o, n) }) || strings.EqualFold(n, s.Address) {
			continue
		}
		names = append(names, n)
		named[s.Address] = n
	}
	var gvl strings.Builder
	gvl.WriteString("VAR_GLOBAL\n\t(* The step chain *)\n\tcurrentStep : INT;  (* the step it is in *)\n\tnextStep : INT;     (* the step it goes to next *)\n\tstepDelay : TIME;   (* how long it waits before it goes there *)\n\n\t(* The station's signals by name, as IO_Mapping sets them from the Bits list *)\n")
	for _, s := range signals {
		if n, ok := named[s.Address]; ok {
			fmt.Fprintf(&gvl, "\t%s : BOOL; (* %s *)\n", n, comment(s.Description))
		}
	}
	gvl.WriteString("END_VAR\n")
	var io strings.Builder
	io.WriteString("(* Maps the Bits list to the names the program uses: inputs first, outputs after. *)\nPROGRAM IO_Mapping\n")
	for _, s := range signals {
		if n, ok := named[s.Address]; ok && s.Input {
			fmt.Fprintf(&io, "%s := %s;\n", n, s.Address)
		}
	}
	io.WriteString("\n")
	for _, s := range signals {
		if n, ok := named[s.Address]; ok && !s.Input {
			fmt.Fprintf(&io, "%s := %s;\n", s.Address, n)
		}
	}
	io.WriteString("END_PROGRAM\n")
	files := map[string]string{
		"GVL.st":        gvl.String(),
		"IO_Mapping.st": io.String(),
		"StepHandler.st": `(* Moves the step chain on: once nextStep differs from currentStep, it waits for delay
   and then makes nextStep the current step. *)
FUNCTION_BLOCK StepHandler
VAR_IN_OUT
	current : INT;     (* the step the chain is in *)
END_VAR
VAR_INPUT
	next : INT;        (* the step it goes to *)
	delay : TIME;      (* how long it waits first *)
END_VAR
VAR
	timer : TON;
END_VAR

IF current <> next THEN
	timer(IN := TRUE, PT := delay);
	IF timer.Q THEN
		current := next;
		timer(IN := FALSE);
	END_IF
ELSE
	timer(IN := FALSE);
END_IF
END_FUNCTION_BLOCK
`,
		"StepChain.st": `(* The station's sequence: each step sets the outputs it needs, and when it is done, the
   step to go to and how long to wait before that. *)
PROGRAM StepChain

CASE currentStep OF
	0: (* Init *)
		nextStep := 10;
		stepDelay := T#0ms;

	10: (* Base position *)
		;

END_CASE
END_PROGRAM
`,
		"PLC_PRG.st": `PROGRAM PLC_PRG
VAR
	stepHandler : StepHandler;
END_VAR

StepChain();
stepHandler(current := currentStep, next := nextStep, delay := stepDelay);
IO_Mapping();
END_PROGRAM
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			return err
		}
	}
	data, _ := json.MarshalIndent(Meta{Programs: []string{"PLC_PRG"}, Cycle: "T#20ms", Step: "GVL.currentStep"}, "", "  ")
	return os.WriteFile(filepath.Join(dir, metaFile), data, 0o644)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Signals is the I/O table of one of a station's PLCs, as the Bits list declares it.
func Signals(p model.ScenePLC) []Signal {
	var out []Signal
	for _, s := range p.Inputs {
		out = append(out, Signal{Name: s.Name, Address: s.Address, Variable: s.Variable, Description: s.Description, Input: true, Byte: int(s.Byte), Bit: int(s.Bit)})
	}
	for _, s := range p.Outputs {
		out = append(out, Signal{Name: s.Name, Address: s.Address, Variable: s.Variable, Description: s.Description, Byte: int(s.Byte), Bit: int(s.Bit)})
	}
	return out
}
