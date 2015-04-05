package spc

const (
	VOLL = iota
	VOLR
	PL
	PH
	SRCN
	ADSR1
	ADSR2
	GAIN
	ENVX
	OUTX

	MVOLL = 0xc + 0x10*iota
	MVOLR
	EVOLL
	EVOLR
	KON
	KOF
	FLG
	ENDX

	EFB = 0xd + 0x10*iota
	_
	PMON
	NOV
	EOV
	DIR
	ESA
	EDL

	C0 = 0xf + 0x10*iota
	C1
	C2
	C3
	C4
	C5
	C6
	C7
)
