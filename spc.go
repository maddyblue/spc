package spc

import (
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/mjibson/nsf/cpu6502"
)

type ram struct {
	M [0xffff + 1]byte
	A apu
}

func (r *ram) Read(v uint16) byte {
	switch v {
	//case 0x4015:
	//	return r.A.Read(v)
	default:
		return r.M[v]
	}
}

func (r *ram) Write(v uint16, b byte) {
	r.M[v] = b
	if v&0xf000 == 0x4000 {
		//	r.A.Write(v, b)
	}
}

type apu struct {
}

type SPC struct {
	ContainsID   bool
	Song         string
	Game         string
	Dumper       string
	Comments     string
	DumpDate     string
	Duration     time.Duration
	FadeDuration time.Duration
	Artist       string

	RAM *ram
	CPU *cpu6502.Cpu
}

var ErrFormat = fmt.Errorf("spc: bad format")

func New(r io.Reader) (*SPC, error) {
	var err error
	s := SPC{
		RAM: new(ram),
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(b) < 66048 {
		return nil, ErrFormat
	}
	if string(b[:35]) != "SNES-SPC700 Sound File Data v0.30\x1a\x1a" {
		return nil, ErrFormat
	}
	if b[0x23] == 26 {
		s.ContainsID = true
	} else if b[0x23] == 27 {
	} else {
		return nil, ErrFormat
	}
	if b[0x24] != 30 {
		return nil, ErrFormat
	}
	s.CPU = cpu6502.New(s.RAM)
	s.CPU.PC = uint16(b[0x26])
	s.CPU.PC += uint16(b[0x25]) << 8
	s.CPU.A = b[0x27]
	s.CPU.X = b[0x28]
	s.CPU.Y = b[0x29]
	s.CPU.P = b[0x2A]
	s.CPU.S = b[0x2B]
	s.Song = clean(b[0x2e : 0x2e+32])
	s.Game = clean(b[0x4e : 0x4e+32])
	s.Dumper = clean(b[0x6e : 0x6e+16])
	s.Comments = clean(b[0x7e : 0x7e+32])
	s.DumpDate = clean(b[0x9e : 0x9e+11])
	i, _ := strconv.Atoi(clean(b[0xa9 : 0xa9+3]))
	if i == 0 {
		return nil, ErrFormat
	}
	s.Duration = time.Second * time.Duration(i)
	i, err = strconv.Atoi(clean(b[0xac : 0xac+5]))
	if err != nil {
		return nil, ErrFormat
	}
	s.FadeDuration = time.Millisecond * time.Duration(i)
	s.Artist = clean(b[0xb1 : 0xb1+32])
	if n := copy(s.RAM.M[:], b[0x100:0x100+65536]); n != 65536 {
		return nil, ErrFormat
	}
	dsp := b[0x10100 : 0x10100+128]
	_ = dsp

	//s.CPU.T = &s
	//s.ram.A.Init()
	//s.CPU.Run()
	return &s, nil
}

func clean(b []byte) string {
	s := strings.TrimSpace(string(b))
	return strings.Split(s, "\x00")[0]
}

func (s *SPC) Init() {}
