package machine

import "strings"

// Sample is one of the example programs printed in the manual.
type Sample struct {
	Name    string
	Usage   string
	Listing string
}

// Program parses the sample's listing.
func (s Sample) Program() *[Steps]byte {
	p, err := ParseProgram(strings.NewReader(s.Listing))
	if err != nil {
		panic("bad sample " + s.Name + ": " + err.Error())
	}
	return p
}

// Samples are transcribed from the manual. Obvious misprints have been
// corrected (e.g. Creepy Crawler II step 08 is OTA = 0B, not 0A; Enter a
// Letter B step 02 is INP.0 = 70, not 50).
var Samples = []Sample{
	{"Creepy Crawler", "Press E. The little man buzzes about the screen.", `
00 60 3A  LDV.0.3A  little man into Reg. 0
02 61 0C  LDV.1.0C  blank into Reg. 1
04 6B 00  LDV.B.00  position 00
06 C0     OUT.0
07 C1     OUT.1
08 05     SIG       one second buzz
09 08     RND
10 BB     UNP.B     random position into Reg. B
11 12 06  GTO.06
`},
	{"Creepy Crawler II", "Press E. An all-star cast of thousands.", `
00 62 13  LDV.2.13
02 08     RND
03 BB     UNP.B     random position (B), random digit (C)
04 9C     LDA.C
05 E2     ADD.2
06 AC     STO.C     C = 13 + digit: a step in the symbol table
07 09     MOV       load symbol from that step
08 0B     OTA
09 05     SIG
10 00     NOP
11 12 02  GTO.02
13 32
14 33
15 3A
16 34
17 35
18 37
19 3D
20 3E
21 36
22 3C
`},
	{"Addition A", "Press E, then two digits. The sum appears.", `
00 70     INP.0
01 04     INA
02 E0     ADD.0
03 B1     UNP.1
04 6B 00  LDV.B.00
06 C1     OUT.1
07 C2     OUT.2
08 12 00  GTO.00
`},
	{"Addition B", "Press E, then two digits. The whole problem appears.", `
00 70     INP.0
01 04     INA
02 6B 00  LDV.B.00
04 C0     OUT.0
05 63 10  LDV.3.10
07 C3     OUT.3
08 0B     OTA
09 63 2B  LDV.3.2B
11 C3     OUT.3
12 E0     ADD.0
13 B1     UNP.1
14 C1     OUT.1
15 C2     OUT.2
16 12 00  GTO.00
`},
	{"Addition C", "Press E, then two digits; the problem builds as you type.", `
00 6B 00  LDV.B.00
02 70     INP.0
03 C0     OUT.0
04 63 10  LDV.3.10
06 C3     OUT.3
07 04     INA
08 0B     OTA
09 63 2B  LDV.3.2B
11 C3     OUT.3
12 E0     ADD.0
13 B1     UNP.1
14 C1     OUT.1
15 C2     OUT.2
16 70     INP.0     pause until the next problem
17 6C 0B  LDV.C.0B
19 67 0C  LDV.7.0C
21 9C     LDA.C
22 64 00  LDV.4.00
24 02     DEC
25 C7     OUT.7     erase with blanks
26 24 24  BNE.4.24
28 6B 00  LDV.B.00
30 12 03  GTO.03
`},
	{"Multiplication", "Press E, then multiplicand and multiplier (2-9).", `
00 6B 00  LDV.B.00
02 70     INP.0
03 C0     OUT.0
04 66 29  LDV.6.29
06 C6     OUT.6
07 71     INP.1
08 C1     OUT.1
09 67 2B  LDV.7.2B
11 C7     OUT.7
12 90     LDA.0
13 E0     ADD.0
14 A2     STO.2
15 91     LDA.1
16 02     DEC
17 A1     STO.1
18 63 01  LDV.3.01
20 33 25  BEQ.3.25
22 92     LDA.2
23 12 13  GTO.13
25 92     LDA.2
26 B4     UNP.4
27 C4     OUT.4
28 C5     OUT.5
29 12 00  GTO.00
`},
	{"Division", "Press E, then dividend and divisor (dividend larger).", `
00 63 00  LDV.3.00
02 6B 00  LDV.B.00
04 70     INP.0
05 C0     OUT.0
06 69 2A  LDV.9.2A
08 C9     OUT.9
09 71     INP.1
10 C1     OUT.1
11 6A 2B  LDV.A.2B
13 CA     OUT.A
14 91     LDA.1
15 D0     SUB.0
16 A0     STO.0
17 93     LDA.3
18 03     INC
19 A3     STO.3
20 90     LDA.0
21 13 40  BRZ.40
23 91     LDA.1
24 50 28  BLS.0.28
26 12 15  GTO.15
28 93     LDA.3
29 B4     UNP.4
30 C4     OUT.4
31 C5     OUT.5
32 66 10  LDV.6.10
34 67 13  LDV.7.13
36 C6     OUT.6
37 C7     OUT.7
38 12 00  GTO.00
40 93     LDA.3
41 B4     UNP.4
42 C4     OUT.4
43 C5     OUT.5
44 66 0C  LDV.6.0C
46 67 0C  LDV.7.0C
48 C6     OUT.6
49 C7     OUT.7
50 12 00  GTO.00
`},
	{"Area (Subroutine)", "Press E, base, height, then 1 = rectangle or 2 = triangle.", `
00 6B 00  LDV.B.00
02 61 01  LDV.1.01
04 62 02  LDV.2.02
06 73     INP.3     base
07 74     INP.4     height
08 04     INA       1 or 2
09 31 13  BEQ.1.13
11 32 20  BEQ.2.20
13 14 66  GTS.66
15 B5     UNP.5
16 C5     OUT.5
17 C6     OUT.6
18 12 58  GTO.58
20 14 66  GTS.66
22 00     NOP
23 A3     STO.3
24 64 02  LDV.4.02
26 67 00  LDV.7.00
28 94     LDA.4
29 D3     SUB.3
30 A3     STO.3
31 97     LDA.7
32 03     INC
33 A7     STO.7
34 93     LDA.3
35 13 54  BRZ.54
37 94     LDA.4
38 53 42  BLS.3.42
40 12 29  GTO.29
42 97     LDA.7
43 B8     UNP.8
44 C8     OUT.8
45 C9     OUT.9
46 6E 10  LDV.E.10
48 6F 13  LDV.F.13
50 CE     OUT.E
51 CF     OUT.F
52 12 58  GTO.58
54 97     LDA.7
55 B8     UNP.8
56 C8     OUT.8
57 C9     OUT.9
58 6E 0C  LDV.E.0C
60 6F 0C  LDV.F.0C
62 CE     OUT.E
63 CF     OUT.F
64 12 00  GTO.00
66 93     LDA.3     multiply subroutine
67 E3     ADD.3
68 A5     STO.5
69 94     LDA.4
70 02     DEC
71 A4     STO.4
72 31 77  BEQ.1.77
74 95     LDA.5
75 12 67  GTO.67
77 95     LDA.5
78 07     RET
`},
	{"Flash Card", "Press E, then the two-digit answer (07 for 7).", `
00 6A 0C  LDV.A.0C
02 68 10  LDV.8.10
04 69 2B  LDV.9.2B
06 6C 2D  LDV.C.2D
08 6D 17  LDV.D.17
10 08     RND
11 B0     UNP.0
12 6B 00  LDV.B.00
14 C0     OUT.0
15 C8     OUT.8
16 C1     OUT.1
17 00     NOP
18 C9     OUT.9
19 90     LDA.0
20 E1     ADD.1
21 A2     STO.2
22 73     INP.3
23 C3     OUT.3
24 74     INP.4
25 C4     OUT.4
26 83     PAK.3
27 CA     OUT.A
28 32 45  BEQ.2.45
30 CC     OUT.C
31 CD     OUT.D
32 6B 04  LDV.B.04
34 73     INP.3
35 C3     OUT.3
36 CA     OUT.A
37 CA     OUT.A
38 00     NOP
39 CA     OUT.A
40 CA     OUT.A
41 6B 05  LDV.B.05
43 12 24  GTO.24
45 05     SIG       the musical reward
46 14 70  GTS.70
48 05     SIG
49 05     SIG
50 05     SIG
51 14 70  GTS.70
53 05     SIG
54 14 70  GTS.70
56 14 70  GTS.70
58 05     SIG
59 14 70  GTS.70
61 05     SIG
62 6B 00  LDV.B.00
64 CA     OUT.A
65 9B     LDA.B
66 13 10  BRZ.10
68 12 64  GTO.64
70 67 00  LDV.7.00  rest (delay subroutine)
72 6E 75  LDV.E.75
74 9E     LDA.E
75 00     NOP
76 02     DEC
77 27 75  BNE.7.75
79 07     RET
`},
	{"Enter a Letter A", "Press E. The letter H appears on the screen.", `
00 6B 00  LDV.B.00
02 60 1D  LDV.0.1D  H into Reg. 0
04 C0     OUT.0
05 71     INP.1     pause
`},
	{"Enter a Letter B", "Press E, then any key; it appears on the screen.", `
00 6B 00  LDV.B.00
02 70     INP.0
03 C0     OUT.0
04 71     INP.1     pause
`},
	{"Enter a Letter C", "Press E, then any key; it goes to the screen through the accumulator.", `
00 6B 00  LDV.B.00
02 04     INA
03 0B     OTA
04 71     INP.1     pause
`},
	{"Six Letter Guess", "Press E, type a 6-letter word, then your opponent guesses letters.", `
00 67 01  LDV.7.01
02 6B 00  LDV.B.00
04 79     INP.9
05 61 27  LDV.1.27
07 C1     OUT.1
08 7A     INP.A
09 62 27  LDV.2.27
11 C2     OUT.2
12 7C     INP.C
13 63 27  LDV.3.27
15 C3     OUT.3
16 7D     INP.D
17 64 27  LDV.4.27
19 C4     OUT.4
20 7E     INP.E
21 65 27  LDV.5.27
23 C5     OUT.5
24 7F     INP.F
25 66 27  LDV.6.27
27 C6     OUT.6
28 6B 00  LDV.B.00
30 C1     OUT.1
31 C2     OUT.2
32 C3     OUT.3
33 00     NOP
34 C4     OUT.4
35 C5     OUT.5
36 C6     OUT.6
37 00     NOP
38 37 00  BEQ.7.00
40 04     INA
41 39 55  BEQ.9.55
43 3A 58  BEQ.A.58
45 3C 61  BEQ.C.61
47 3D 64  BEQ.D.64
49 3E 67  BEQ.E.67
51 3F 70  BEQ.F.70
53 12 71  GTO.71
55 A1     STO.1
56 12 43  GTO.43
58 A2     STO.2
59 12 45  GTO.45
61 A3     STO.3
62 12 47  GTO.47
64 A4     STO.4
65 12 49  GTO.49
67 A5     STO.5
68 12 51  GTO.51
70 A6     STO.6
71 68 27  LDV.8.27
73 98     LDA.8
74 31 28  BEQ.1.28
76 32 28  BEQ.2.28
78 33 28  BEQ.3.28
80 34 28  BEQ.4.28
82 35 28  BEQ.5.28
84 36 28  BEQ.6.28
86 67 2B  LDV.7.2B
88 97     LDA.7
89 05     SIG
90 05     SIG
91 12 28  GTO.28
`},
	{"Message", "Press E, then 1-6 to show a message.", `
00 60 90  LDV.0.90  location table base
02 61 0C  LDV.1.0C
04 04     INA
05 E0     ADD.0
06 AC     STO.C
07 09     MOV
08 AC     STO.C
09 6B 00  LDV.B.00
11 62 11  LDV.2.11
13 63 00  LDV.3.00
15 91     LDA.1
16 0B     OTA
17 92     LDA.2
18 02     DEC
19 A2     STO.2
20 23 15  BNE.3.15
22 6B 00  LDV.B.00
24 09     MOV
25 33 04  BEQ.3.04
27 0B     OTA
28 12 24  GTO.24
30 1D 12 0E 0E 17 00                    HELLO
36 14 20 1F 12 0C 20 0C 0E 17 17 1F 00  TAKE A LOOK
48 13 12 26 20 13 1F 20 25 0E 12 00     REMARKABLE
59 2D 12 11 0C 1B 17 13 0C 07 08 00     NEW FOR 78
70 18 15 12 19 14 16 17 2D 19 0D 00     QUESTIONS?
81 23 17 26 12 0C 25 20 23 1F 00        COME BACK
91 30 36 48 59 70 81                    location table
`},
}
