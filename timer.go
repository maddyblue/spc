package spc

type Timer struct {
	Enabled bool
	Counter byte
	Ticker  byte
	Upto    byte
	Freq    int
}

func NewTimer(freq int) *Timer {
	return &Timer{
		Freq: freq,
	}
}

func (t *Timer) Read() byte {
	c := t.Counter
	t.Counter = 0
	return c
}

func (t *Timer) Set(b byte) {
	t.Upto = b
}

func (t *Timer) Enable(e bool) {
	t.Enabled = e
	if e {
		t.Ticker = 0
		t.Counter = 0
	}
}

func (t *Timer) Tick() {
	n := t.Counter
	if (n & 7) == 0 {
		n -= i
	}
	n--
	t.Counter = n
	if !t.Enabled {
		return
	}
	t.Ticker++
	if t.Ticker == t.Upto {
		t.Ticker = 0
		t.Counter = (t.Counter + 1) & 0xf
	}
}