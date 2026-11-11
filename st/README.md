# st

A compiler and runtime for IEC 61131-3 Structured Text. It is a Go module of its own and uses only the standard library. It follows the 3rd edition of the standard for ST. Where the standard leaves a choice to the implementation, it does what CODESYS does: keywords and names are not case sensitive, integer arithmetic wraps at the width of its type, and a PROGRAM can call another PROGRAM. It knows none of the standard's other languages.

```go
prog, diags := st.Compile(st.Project{Files: files, Programs: []string{"PLC_PRG"}, Cycle: 20 * time.Millisecond})
rt := st.NewRuntime(prog)
rt.Start()
out := rt.Scan(in, now) // one cycle: inputs, forces, programs, forces, outputs
```

Each message from `Compile` has a file, line and column. While any error is left, `Compile` gives no program. Runtime faults stop the PLC with the place they happened and switch every output off. Faults include division by zero, an array index out of its bounds, and a cycle that runs past its watchdog.

## What it supports

- **POUs and declarations:** `PROGRAM`, `FUNCTION_BLOCK` and `FUNCTION`; global variable lists (`VAR_GLOBAL`, which can be qualified by file name, as `GVL.x`); and `TYPE` for structs, enums with or without a base type, and aliases.
- **Variable sections:** `VAR`, `VAR_INPUT`, `VAR_OUTPUT`, `VAR_IN_OUT`, `VAR_TEMP`, `VAR_STAT` and `VAR_EXTERNAL`, with `CONSTANT` and `RETAIN`. Initial values can be given, including array and struct initialisers and `n(x)` repeats.
- **Types:**
  - `BOOL`
  - the integers `SINT` to `ULINT`
  - the bit strings `BYTE` to `LWORD`
  - `REAL` and `LREAL`
  - `TIME`
  - `STRING` and `WSTRING`
  - `ARRAY [a..b, …] OF`
  - structs and enums
  - the instances of function blocks
- **Statements:** `:=`, `IF`, `CASE` (with ranges and lists), `FOR`, `WHILE`, `REPEAT`, `EXIT`, `CONTINUE` and `RETURN`. Calls can use formal arguments, including `=>` and `NOT` on outputs. Single bits are reached as `x.3`.
- **Standard functions:**
  - type conversion (`A_TO_B`, `TO_B`, `TRUNC_B`)
  - numeric: `ABS`, `SQRT`, `LN`, `LOG`, `EXP`, the trigonometric functions and `EXPT`
  - selection: `SEL`, `MAX`, `MIN`, `LIMIT` and `MUX`
  - bit shifts: `SHL`, `SHR`, `ROL` and `ROR`
  - strings: `LEN`, `LEFT`, `RIGHT`, `MID`, `CONCAT`, `INSERT`, `DELETE`, `REPLACE` and `FIND`
- **Standard function blocks:** `TON`, `TOF`, `TP`, `R_TRIG`, `F_TRIG`, `CTU`, `CTD`, `CTUD`, `SR` and `RS`.
- **Direct addresses:** `AT %IXn.m` and `AT %QXn.m`, bits only, on a process image of 64 bytes each way.
- **Online access:** `Variables`, `Read`, `Write`, `Force` and `Release` on paths such as `PLC_PRG.t.Q`, `GVL.x` or `arr[1, 2]`.

## Where it differs from the standard or CODESYS

- `DATE`, `TIME_OF_DAY`, `DATE_AND_TIME`, `LTIME`, `CHAR` and `WCHAR` are reported as not supported yet.
- Direct addresses are bits only. There are no `%IW` or `%MD` addresses.
- There is no object orientation (`METHOD`, `PROPERTY`, `INTERFACE` or `EXTENDS`), and there are no pointers or references.
- Conversions are implicit only where a type widens without loss. Any other conversion needs `X_TO_Y`, and the error message names the one to use.
- A force applies at the start and the end of each cycle, as in CODESYS. The program sees the forced value, but it can still change the variable during the cycle.
- `F_TRIG` remembers its last input from its initial value, so it does not fire in the first cycle.
- PLCopen XML import and the graphical languages (LD, FBD, SFC) are out of scope.
