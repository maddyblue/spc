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

	RAM [0xffff + 1]byte
	CPU *cpu6502.Cpu
	DSP [128]byte

	// Registers

	DSPAddr byte
	Timer   [3]*Timer
	
	//
	
	// todo: maybe use int32 instead of int because of c's int size?
	// be sure to check all uses of int
	
	phase int
	every_other_sample bool
	new_kon, kon byte
	t_koff byte
	counters [4]uint16
	counter_select [32]*uint16
	noise int
	voices [voice_count]*voice_t
	regs [register_count]uint8
}

const (
	voice_count = 8
	register_count = 128
	brr_buf_size = 12
)

type voice_t struct {
		 buf [brr_buf_size*2]int;// decoded samples (twice the size to simplify wrap handling)
		 buf_pos []int;           // place in buffer where next samples will be decoded
		 interp_pos int;         // relative fractional position in sample (0x1000 = 1.0)
		 brr_addr int;           // address of current BRR block
		 brr_offset int;         // current decoding offset in BRR block
		 kon_delay int;          // KON delay/current setup phase
		env_mode env_mode_t;
		 env int;                // current envelope level
		 hidden_env int;         // used by GAIN mode 7, very obscure quirk
		 volume [2]int;         // copy of volume from DSP registers, with surround disabled
		 enabled int;            // -1 if enabled, 0 if muted
}

func (s *SPC) dir() byte {
	return s.RAM[
}

var ErrFormat = fmt.Errorf("spc: bad format")

func New(r io.Reader) (*SPC, error) {
	var err error
	s := SPC{}
	s.Timer[0] = NewTimer(8000)
	s.Timer[1] = NewTimer(8000)
	s.Timer[2] = NewTimer(64000)
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
	s.CPU = cpu6502.New(&s)
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
	if n := copy(s.RAM[:], b[0x100:0x100+65536]); n != 65536 {
		return nil, ErrFormat
	}
	dsp := b[0x10100 : 0x10100+128]
	_ = dsp

	//s.CPU.T = &s
	//s.CPU.Run()
	return &s, nil
}

func clean(b []byte) string {
	s := strings.TrimSpace(string(b))
	return strings.Split(s, "\x00")[0]
}

func (s *SPC) Read(v uint16) byte {
	switch v {
	case 0xf0:
	case 0xf1:
	case 0xf2:
		return s.DSPAddr
	case 0xf3:
		return s.DSP[s.DSPAddr]
	case 0xf4:
	case 0xf5:
	case 0xf6:
	case 0xf7:
	// regular memory
	//case 0xf8:
	//case 0xf9:
	case 0xfa:
	case 0xfb:
	case 0xfc:
	case 0xfd:
		return s.Timer[0].Read()
	case 0xfe:
		return s.Timer[1].Read()
	case 0xff:
		return s.Timer[2].Read()
	default:
		return s.RAM[v]
	}
	return 0
}

func (s *SPC) Write(v uint16, b byte) {
	switch v {
	case 0xf0:
	case 0xf1:
		s.Timer[0].Enable(b&1 == 1)
		s.Timer[1].Enable(b&2 == 1)
		s.Timer[2].Enable(b&4 == 1)
	case 0xf2:
		s.DSPAddr = b
	case 0xf3:
		s.DSP[s.DSPAddr] = b
	case 0xf4:
	case 0xf5:
	case 0xf6:
	case 0xf7:
	// regular memory
	//case 0xf8:
	//case 0xf9:
	case 0xfa:
		s.Timer[0].Set(b)
	case 0xfb:
		s.Timer[1].Set(b)
	case 0xfc:
		s.Timer[2].Set(b)
	case 0xfd:
	case 0xfe:
	case 0xff:
	default:
		s.RAM[v] = b
	}
}
