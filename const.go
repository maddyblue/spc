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
	NON
	EOV
	DIR
	ESA
	EDL

	FIR = 0xf
)
