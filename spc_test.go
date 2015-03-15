package spc

import (
	"os"
	"testing"
	"time"
)

func TestSPC(t *testing.T) {
	f, err := os.Open("soe-02.spc")
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	if s.Song != "Menu" || s.Artist != "Jeremy Soule" {
		t.Fatal("bad")
	}
	if s.Duration != time.Second*61 || s.FadeDuration != time.Second*8 {
		t.Fatal("bad time")
	}
}
