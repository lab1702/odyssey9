# Odyssey² Computer Intro! emulator (Videopac 9)

An emulator of the Magnavox Odyssey² with the **Computer Intro!** cartridge
(Philips Videopac 9) inserted, written in Go with
[Ebitengine](https://ebitengine.org) so it runs on Windows, macOS and Linux.

The window shows a TV screen above a copy of the console keyboard as drawn in
the manual. Click the keys or type on your own keyboard.

The cartridge turns the console into a small teaching computer, and that
computer is what this emulator implements. It is built from the manual
(`computer-intro.txt` / the PDF) and needs no ROM files:

* 16 registers (0–F), an accumulator, a program counter and a subroutine
  return register
* 100 program steps (00–99) that you enter in hex (machine code) or assembler
* an 11-position output line on the TV, with the 63 symbols from the manual's
  "Computer Symbols" chart
* all 8 operating modes: Command, Execution, Continue, Display, Program,
  Assembler, Hex Input, Input and Roll
* the full instruction set, including HLT for debugging
* the 4 built-in games: Creepy Crawler, Flash Card, Computer Telepathy and
  Between the Sheets
* the beep, one-second buzz and other sounds

## Building

You need Go 1.22 or newer.

```sh
go build -o odyssey9 .        # odyssey9.exe on Windows
./odyssey9
```

* **Windows:** no extra setup. You can also cross-compile from any OS with
  `GOOS=windows GOARCH=amd64 go build -o odyssey9.exe .`
* **macOS:** install the Xcode command line tools (`xcode-select --install`).
  Build on a Mac, because macOS builds need cgo.
* **Linux:** install the X11, OpenGL and ALSA development headers first.
  On Debian/Ubuntu:

  ```sh
  sudo apt install gcc libx11-dev libxrandr-dev libxcursor-dev libxinerama-dev \
      libxi-dev libxxf86vm-dev libgl1-mesa-dev libasound2-dev pkg-config
  ```

To run the tests: `go test ./machine`. They run the programs from the manual
and check what ends up on the screen.

Command-line options:

| Option | Meaning |
|---|---|
| `-load FILE` | load a program listing at start-up |
| `-file FILE` | file used by SAVE / LOAD (default `~/odyssey9-program.txt`) |
| `-monitor` | start with the monitor panel open |
| `-mute` | no sound |

## Using it

Follow the manual. At power-on the screen says **SELECT GAME**:

* **1–4** start the built-in games.
* **RESET (▲)** gives you **COMMAND**, and from there:
  * `E` executes the program.
  * `C` continues after a HLT.
  * `D` is Display mode: `0`–`F` show a register, `P` the program counter,
    `S` the subroutine register and `X` the accumulator.
  * `P` is Program mode: `M` for hex or `A` for assembler, then `I` to input
    or `R` to roll.
* In **Input** mode, type `6B`, `ENTER`, `00`, `ENTER` in hex, or
  `LDV.B.00`, `ENTER` in assembler. `CLEAR` steps back one mode.
* In **Roll** mode, `U` and `D` move through the steps. `CLEAR` returns to
  Program mode, so you can then press `M`/`A` and `I` to overwrite a step.
* Switching the power off (POWER button / F8) erases the program, just like
  the real console.

### PC keyboard

| PC key | Odyssey² key |
|---|---|
| letters, digits, `.` `?` `+` `-` `=`, space | the same key |
| `*` | × |
| `/` | ÷ |
| `[` / `]` | YES / NO |
| Enter | ENTER |
| Delete | CLEAR |
| Esc | RESET |
| Backspace | erases the last character you typed in Input mode (otherwise CLEAR) |

### Emulator controls

There is a row of buttons under the TV, and each one also has a function key:

* **POWER (F8)** switches the console off and on.
* **SPEED (F5)** sets how fast programs run:
  * SLOW: 4 steps/s, for watching a program work
  * NORMAL: 1000 steps/s
  * FAST
* **PROGRAMS (F4)** loads any of the 12 programs printed in the manual.
* **SAVE (F2)** / **LOAD (F3)** write and read your program as a text
  listing. You can also drop a listing file onto the window.
* **MONITOR (F6)** opens a side panel showing the registers, the accumulator,
  the program counter and a disassembly of program memory.
* **F7** colours the characters the way the manual's symbol chart does.
* **HELP (F1)** shows a quick reference.

The `examples/` folder holds the manual's programs as listing files.

### Listing format

Each line has a decimal step number, then one or more two-digit hex bytes.
Everything after the bytes is ignored, so the manual's own tables load as they
are:

```
04   6B 00  LDV.B.00   Set output position to 00
30   1D 12 0E 0E 17 00 HELLO
```

## How the computer behaves

The manual doesn't spell out every detail, so some behaviour was worked out
from its example programs. All 12 examples give the results the manual
describes. For example, `9 ÷ 2` shows `04+R`, Message 2 shows
`TAKE A LOOK` and the Area program computes `8 × 3 = 24`.

* **Step numbers are decimal.** `GTO.15` is coded `12 15`, and `ADD`, `SUB`,
  `INC` and `DEC` work in decimal. That is why `9+9` unpacks to the digits
  1 and 8, and why Creepy Crawler II's `13 + digit` lands on steps 13–22.
* **`SUB.R`** works out *register − accumulator*. `DEC` takes 00 down to 99.
  `BDB` branches when the accumulator's high digit is 9, and `BDC` when it
  is not 0.
* **`MOV`** loads the accumulator from the step whose number is in register C,
  then advances C by one. That is how the Message program walks through its
  text.
* **`OUT`/`OTA`** write at the position in register B (00–0A), which then
  advances and wraps from 0A back to 00.
* **`INP`/`INA`** show `?` and wait for a key, which beeps. The value stored
  is the key code from the manual's Key Codes table (`YES` = 2C, `NO` = 2D,
  `CLEAR` = 2E, `ENTER` = 2F). Keys pressed while a program isn't waiting are
  ignored.
* **`SIG`** buzzes for one second, and the program pauses while it sounds.
* **`HLT` (FF)** stops with **HALT** shown and puts you in Command mode. Then
  `D` `P` shows the step that holds the FF, and `C` continues from the
  following step.
* Undefined opcodes act as NOP. The manual's rule about adding a NOP after
  three outputs in a row was a hardware timing limit, and the emulator
  doesn't need it.
* The manual doesn't describe the screen layout of the built-in games in full,
  so their exact layout and timing are reconstructions from its text.

## Project layout

| Path | Contents |
|---|---|
| `machine/` | the emulated computer, with no GUI code: CPU and modes (`machine.go`), assembler/disassembler and decimal arithmetic (`asm.go`), games, font, key codes, listing format, sample programs, tests |
| `main.go`, `game.go` | window, TV, buttons, monitor panel, overlays |
| `keyboard.go` | on-screen keyboard and host keyboard mapping |
| `audio.go` | sound synthesis |
| `draw.go` | resolution-independent drawing helpers |
| `script.go` | scripted key input and screenshots, used for testing (`-script`, `-screenshot`) |
