// Package st compiles and runs IEC 61131-3 Structured Text, as a soft PLC does: programs,
// function blocks, functions, global variable lists and data types, the standard functions
// and function blocks, and a runtime that runs a task's program cyclically with a process
// image, and lets a programming tool watch, write and force its variables.
//
// It follows IEC 61131-3 (3rd edition) for Structured Text, and CODESYS where the standard
// leaves things to the implementation: keywords and names are case-insensitive, integer
// arithmetic wraps at the width of its type, and calling a PROGRAM from another one runs it.
// Of the other languages of the standard it knows none.
//
// A program is compiled from a Project, a set of source files:
//
//	prog, diags := st.Compile(project)
//	rt := st.NewRuntime(prog)
//	out := rt.Scan(in, now)
//
// Compile reports every mistake it finds with its file, line and column, and does not give
// a program while there is one.
package st
