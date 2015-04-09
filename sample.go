package spc

func (s *SPC) Run(clock_count int) {
	var v_regs, dir []byte
	REG := func(addr uint16) byte {
		return s.RAM[addr]
	}
	VREG := func(addr uint16) byte {
		return v_regs[addr]
	}
	GET_LE16A := func(v []byte) uint16 {
		return uint16(v[0]) << 8 + uint16(v[1])
	}
	SAMPLE_PTR := func(i int) uint16 {
		return GET_LE16A(dir[int(VREG(v_srcn)) * 4 + i * 2:])
	}

	var new_phase int = s.phase + clock_count;
	var count int = new_phase >> 5;
	s.phase = new_phase & 31;
	if  count == 0 {
		return
	}
	
	dir = s.RAM[uint16(REG(DIR)) * 0x100:]
	  slow_gaussian := (REG(PMON) >> 1) | REG(NON);
	  noise_rate  := REG(FLG) & 0x1F;
	
	// Global volume
	mvoll := int8(REG(MVOLL))
	 mvolr  := int8(REG(MVOLR))
	if  mvoll * mvolr < s.surround_threshold {
		mvoll = -mvoll; // eliminate surround
		}
	
	
	for	{
		// KON/KOFF reading
		s.every_other_sample = !s.every_other_sample
		if  s.every_other_sample 	{
			s.new_kon &= ^s.kon;
			s.kon    = s.new_kon;
			s.t_koff = REG(KOFF); 
		}
		
		s.run_counter( 1 );
		s.run_counter( 2 );
		s.run_counter( 3 );
		
		// Noise
		if  !READ_COUNTER( noise_rate ) 	{
			 feedback  := (s.noise << 13) ^ (s.noise << 14);
			s.noise = (feedback & 0x4000) ^ (s.noise >> 1);
		}
		
		// Voices
		var pmon_input int = 0;
		var main_out_l int = 0;
		var main_out_r int = 0;
		var echo_out_l int = 0;
		var echo_out_r int = 0;
		v := s.voices[:];
		v_regs = s.regs[:];
		var vbit int = 1;
		for	{
			var brr_header int = ram [v.brr_addr];
			var kon_delay int = v.kon_delay;
			
			// Pitch
			var pitch int = GET_LE16A( &VREG(pitchl) ) & 0x3FFF;
			if  REG(PMON) & vbit {
				pitch += ((pmon_input >> 5) * pitch) >> 10;
			}
			
			// KON phases
			kon_delay--
			if  kon_delay >= 0 	{
				v.kon_delay = kon_delay;
				
				// Get ready to start BRR decoding on next sample
				if  kon_delay == 4 	{
					v.brr_addr   = SAMPLE_PTR( 0 );
					v.brr_offset = 1;
					v.buf_pos    = v.buf[:]
					brr_header    = 0; // header is ignored on this sample
				}
				
				// Envelope is never run during KON
				v.env        = 0;
				v.hidden_env = 0;
				
				// Disable BRR decoding until last three samples
				if kon_delay & 3 {
					v.interp_pos = 0x4000
				} else {
					v.interp_pos = 0
				}
				
				// Pitch is never added during KON
				pitch = 0;
			}
			
			env := v.env
			
			// Gaussian interpolation
			{
				var output int = 0;
				VREG(envx) = (uint8_t) (env >> 4);
				if  env 	{
					// Make pointers into gaussian based on fractional position between samples
					// todo: determine if this unsigned cast does anything
					// or maybe offset can be made an unsigned instead?
					//var offset int = (unsigned) v.interp_pos >> 3 & 0x1FE;
					offset := v.interp_pos >> 3 & 0x1FE
					fwd := interleved_gauss[offset:]
					rev := interleved_gauss[510 - offset:] // mirror left half of gaussian
					
					//var const int* in = &v.buf_pos [(unsigned) v.interp_pos >> 12];
					in := v.buf_pos[v.interp_pos >> 12]
					
					// 99%
					if  (slow_gaussian & vbit) == 0 	{
						// Faster approximation when exact sample value isn't necessary for pitch mod
						output = (fwd [0] * in [0] +
						          fwd [1] * in [1] +
						          rev [1] * in [2] +
						          rev [0] * in [3]) >> 11;
						output = (output * env) >> 11;
					}	else	{
						// todo: this conversion might be different than the C
						output = int(int16(s.noise * 2))
						if  (REG(NON) & vbit) == 0	{
							output  = (fwd [0] * in [0]) >> 11;
							output += (fwd [1] * in [1]) >> 11;
							output += (rev [1] * in [2]) >> 11;
							output = int(int16(output))
							output += (rev [0] * in [3]) >> 11;
							
							output = CLAMP16( output );
							output &= ^1;
						}
						output = (output * env) >> 11 & ^1;
					}
					
					// Output
					l := output * v.volume [0];
					r := output * v.volume [1];
					
					main_out_l += l;
					main_out_r += r;
					
					if  (REG(EON) & vbit) != 0 	{
						echo_out_l += l;
						echo_out_r += r;
					}
				}
				
				pmon_input = output;
				VREG(outx) = (uint8_t) (output >> 8);
			}
			
			// Soft reset or end of sample
			if  REG(FLG) & 0x80 || (brr_header & 3) == 1 	{
				v.env_mode = env_release;
				env         = 0;
			}
			
			if  s.every_other_sample	{
				// KOFF
				if  s.t_koff & vbit {
					v.env_mode = env_release;
				}
				
				// KON
				if  s.kon & vbit 	{
					v.kon_delay = 5;
					v.env_mode  = env_attack;
					REG(ENDX) &= ^vbit;
				}
			}
			
			// Envelope
			if  !v.kon_delay 	{
				// 97%
				if  v.env_mode == env_release  	{
					env -= 0x8;
					v.env = env;
					if  env <= 0 	{
						v.env = 0;
						goto skip_brr; // no BRR decoding for you!
					}
				// 3%
				}	else 	{
					var rate int;
					var adsr0 int = VREG(adsr0);
					var env_data int = VREG(adsr1);
					// 97% ADSR
					if  adsr0 >= 0x80  	{ 
						// 89%
						if  v.env_mode > env_decay 	{
							env--;
							env -= env >> 8;
							rate = env_data & 0x1F;
							
							// optimized handling
							v.hidden_env = env;
							if  READ_COUNTER( rate )  {
								goto exit_env;
							}
							v.env = env;
							goto exit_env;
						}	else if  v.env_mode == env_decay 	{
							env--;
							env -= env >> 8;
							rate = (adsr0 >> 3 & 0x0E) + 0x10;
						// env_attack
						}	else 	{
							rate = (adsr0 & 0x0F) * 2 + 1;
							if rate < 31 {
								env += 0x20
							} else {
								env += 0x400;
							}
						}
					// GAIN
					}	else 	{
					
						var mode int;
						env_data = VREG(gain);
						mode = env_data >> 5;
						// direct
						if  mode < 4  	{
							env = env_data * 0x10;
							rate = 31;
						}	else	{
							rate = env_data & 0x1F;
							// 4: linear decrease
							if  mode == 4  {
								env -= 0x20;
							// 5: exponential decrease
							} else if  mode < 6  	{
								env--;
								env -= env >> 8;
							// 6,7: linear increase
							} else {
								env += 0x20;
								if mode > 6 && v.hidden_env >= 0x600 {
									env += 0x8 - 0x20; // 7: two-slope linear increase
								}
							}
						}
					}
					
					// Sustain level
					if  (env >> 8) == (env_data >> 5) && v.env_mode == env_decay { 
						v.env_mode = env_sustain;
					}
					
					v.hidden_env = env;
					
					// unsigned cast because linear decrease going negative also triggers this
					if  env > 0x7FF 	{
						if env < 0 {
							env = 0
						} else {
							env = 0x7ff
						}
						if  v.env_mode == env_attack {
							v.env_mode = env_decay;
						}
					}
					
					if  !READ_COUNTER( rate ) {
						v.env = env; // nothing else is controlled by the counter
					}
				}
			}
		exit_env:
			
			{
				// Apply pitch
				var old_pos int = v.interp_pos;
				var interp_pos int = (old_pos & 0x3FFF) + pitch;
				if  interp_pos > 0x7FFF {
					interp_pos = 0x7FFF;
				}
				v.interp_pos = interp_pos;
				
				// BRR decode if necessary
				if  old_pos >= 0x4000 	{
					// Arrange the four input nybbles in 0xABCD order for easy decoding
					var nybbles int = ram [(v.brr_addr + v.brr_offset) & 0xFFFF] * 0x100 +
							ram [(v.brr_addr + v.brr_offset + 1) & 0xFFFF];
					
					// Advance read position
					var   brr_block_size int = 9;
					var brr_offset int = v.brr_offset;
					brr_offset += 2
					if  brr_offset >= brr_block_size 	{
						// Next BRR block
						var brr_addr int = (v.brr_addr + brr_block_size) & 0xFFFF;
						assert( brr_offset == brr_block_size );
						if  brr_header & 1 	{
							brr_addr = SAMPLE_PTR( 1 );
							if  !v.kon_delay {
								REG(ENDX) |= vbit;
							}
						}
						v.brr_addr = brr_addr;
						brr_offset  = 1;
					}
					v.brr_offset = brr_offset;
					
					// Decode
					
					// 0: >>1  1: <<0  2: <<1 ... 12: <<11  13-15: >>4 <<11
					scale := brr_header >> 4;
					right_shift := shifts [scale];
					left_shift  := shifts [scale + 16];
					
					// Write to next four samples in circular buffer
					pos := v.buf_pos[:]
					
					// Decode four samples
					for  end := 0; end < 4; pos, nybbles = pos[1:], nybbles << 4 {
						// Extract upper nybble and scale appropriately
						var s int = ((nybbles & 0xffff) >> right_shift) << left_shift;
						
						// Apply IIR filter (8 is the most commonly used)
						 filter := brr_header & 0x0C;
						p1 := pos [brr_buf_size - 1];
						p2 := pos [brr_buf_size - 2] >> 1;
						if  filter >= 8 	{
							s += p1;
							s -= p2;
							// s += p1 * 0.953125 - p2 * 0.46875
							if  filter == 8  	{
								s += p2 >> 4;
								s += (p1 * -3) >> 6;
							// s += p1 * 0.8984375 - p2 * 0.40625
							}	else 	{
								s += (p1 * -13) >> 7;
								s += (p2 * 3) >> 4;
							}
						// s += p1 * 0.46875
						} else if  filter  	{
							s += p1 >> 1;
							s += (-p1) >> 5;
						}
						
						// Adjust and write sample
						s = CLAMP16( s );
						s = (int16_t) (s * 2);
						pos[0] = s
						pos [brr_buf_size] =  s; // second copy simplifies wrap-around
					}
					
					if  pos >= &v.buf [brr_buf_size] {
						pos = v.buf;
					}
					v.buf_pos = pos;
				}
			}
skip_brr:
			// Next voice
			vbit <<= 1;
			v_regs = v_regs[0x10:]
			v++;
			if  vbit >= 0x100  {
				break;
			}
		}
		
		// Echo position
		var echo_offset int = s.echo_offset;
		echo_ptr := ram [(REG(ESA) * 0x100 + echo_offset) & 0xFFFF:];
		if  !echo_offset {
			s.echo_length = (REG(EDL) & 0x0F) * 0x800;
		}
		echo_offset += 4;
		if  echo_offset >= s.echo_length {
			echo_offset = 0
		}
		s.echo_offset = echo_offset;
		
		// FIR
		var echo_in_l int = GET_LE16SA( echo_ptr + 0 );
		var echo_in_r int = GET_LE16SA( echo_ptr + 2 );
		
		echo_hist_pos := s.echo_hist_pos
		if len(echo_hist_pos) <= 1 {
			echo_hist_pos = s.echo_hist[:]
		} else {
			echo_hist_pos = echo_hist_pos[1:]
		}
		s.echo_hist_pos = echo_hist_pos;
		echo_hist_pos [8] [0] = echo_in_l;
		echo_hist_pos [0] [0] = 	echo_hist_pos [8] [0] 
		echo_hist_pos [8] [1] = echo_in_r;
		echo_hist_pos [0] [1] = echo_hist_pos [8] [1] 
		
		CALC_FIR_ := func(i, in int) int {
			return in * int(REG(FIR + i * 0x10))
		}
		CALC_FIR := func(i, ch int) int {
			return CALC_FIR_(i, echo_hist_pos[i+1][ch])
		}
		for i := 0; i < 7; i++ {
			echo_in_l += CALC_FIR( i, 0 );
			echo_in_r += CALC_FIR( i, 1 );
		}
		
		// Echo out
		if  (REG(FLG) & 0x20) == 0 {
			var l int = (echo_out_l >> 7) + ((echo_in_l * int(REG(EFB))) >> 14);
			var r int = (echo_out_r >> 7) + ((echo_in_r * int(REG(EFB))) >> 14);
			
			/*
			// just to help pass more validation tests
			#if SPC_MORE_ACCURACY
				l &= ~1;
				r &= ~1;
			#endif
			*/
			
			l = CLAMP16( l );
			r = CLAMP16( r );
			
			SET_LE16A( echo_ptr + 0, l );
			SET_LE16A( echo_ptr + 2, r );
		}
		
		// Sound out
		var l int = (main_out_l * mvoll + echo_in_l * int(REG(EVOLL))) >> 14;
		var r int = (main_out_r * mvolr + echo_in_r * int(REG(EVOLR))) >> 14;
		
		l = CLAMP16( l );
		r = CLAMP16( r );
		
		if  (REG(FLG) & 0x40) 	{
			l = 0;
			r = 0;
		}
		
		sample_t* out = s.out;
		WRITE_SAMPLES( l, r, out );
		s.out = out;
		
		count--
		if count == 0 {
			break
		}
	}
}

func (s *SPC) run_counter(i int) {
	n := s.counters[i]
	if  n & 7 == 0 {
		n--
		n -= 6 - i
	} else {
		n--
	}
	s.counters [i] = n
}

func (s *SPC) READ_COUNTER(rate byte) bool {
	return (*m.counter_select [rate] & s.counter_mask [rate]) != 0
}

var counter_mask  = [32]uint{
7 ,
4095 ,
4095 ,
2047 ,
2047 ,
2047 ,
1023 ,
1023 ,
1023 ,
511 ,
511 ,
511 ,
255 ,
255 ,
255 ,
127 ,
127 ,
127 ,
63 ,
63 ,
63 ,
31 ,
31 ,
31 ,
15 ,
15 ,
15 ,
7 ,
7 ,
7 ,
1 ,
0 ,
}

func (s *SPC) init_counter() {
	// counters start out with this synchronization
	s.counters [0] =     1;
	s.counters [1] =     0;
	// TODO: what is this? counters is "unsigned" in the c file
	//s.counters [2] = -0x20u;
	s.counters [3] =  0x0B;
	
	n := 2
	for i := 1; i < 32; i++ {
		s.counter_select [i] = &s.counters [n];
		n--
		if n == 0 {
			n = 3
		}
	}
	s.counter_select [ 0] = &s.counters [0]
	s.counter_select [30] = &s.counters [2]
}

// Interleved gauss table
// interleved_gauss [i] = gauss [(i & 1) * 256 + 255 - (i >> 1 & 0xFF)]
var interleved_gauss=  [512]uint16 {
 370,1305, 366,1305, 362,1304, 358,1304, 354,1304, 351,1304, 347,1304, 343,1303,
 339,1303, 336,1303, 332,1302, 328,1302, 325,1301, 321,1300, 318,1300, 314,1299,
 311,1298, 307,1297, 304,1297, 300,1296, 297,1295, 293,1294, 290,1293, 286,1292,
 283,1291, 280,1290, 276,1288, 273,1287, 270,1286, 267,1284, 263,1283, 260,1282,
 257,1280, 254,1279, 251,1277, 248,1275, 245,1274, 242,1272, 239,1270, 236,1269,
 233,1267, 230,1265, 227,1263, 224,1261, 221,1259, 218,1257, 215,1255, 212,1253,
 210,1251, 207,1248, 204,1246, 201,1244, 199,1241, 196,1239, 193,1237, 191,1234,
 188,1232, 186,1229, 183,1227, 180,1224, 178,1221, 175,1219, 173,1216, 171,1213,
 168,1210, 166,1207, 163,1205, 161,1202, 159,1199, 156,1196, 154,1193, 152,1190,
 150,1186, 147,1183, 145,1180, 143,1177, 141,1174, 139,1170, 137,1167, 134,1164,
 132,1160, 130,1157, 128,1153, 126,1150, 124,1146, 122,1143, 120,1139, 118,1136,
 117,1132, 115,1128, 113,1125, 111,1121, 109,1117, 107,1113, 106,1109, 104,1106,
 102,1102, 100,1098,  99,1094,  97,1090,  95,1086,  94,1082,  92,1078,  90,1074,
  89,1070,  87,1066,  86,1061,  84,1057,  83,1053,  81,1049,  80,1045,  78,1040,
  77,1036,  76,1032,  74,1027,  73,1023,  71,1019,  70,1014,  69,1010,  67,1005,
  66,1001,  65, 997,  64, 992,  62, 988,  61, 983,  60, 978,  59, 974,  58, 969,
  56, 965,  55, 960,  54, 955,  53, 951,  52, 946,  51, 941,  50, 937,  49, 932,
  48, 927,  47, 923,  46, 918,  45, 913,  44, 908,  43, 904,  42, 899,  41, 894,
  40, 889,  39, 884,  38, 880,  37, 875,  36, 870,  36, 865,  35, 860,  34, 855,
  33, 851,  32, 846,  32, 841,  31, 836,  30, 831,  29, 826,  29, 821,  28, 816,
  27, 811,  27, 806,  26, 802,  25, 797,  24, 792,  24, 787,  23, 782,  23, 777,
  22, 772,  21, 767,  21, 762,  20, 757,  20, 752,  19, 747,  19, 742,  18, 737,
  17, 732,  17, 728,  16, 723,  16, 718,  15, 713,  15, 708,  15, 703,  14, 698,
  14, 693,  13, 688,  13, 683,  12, 678,  12, 674,  11, 669,  11, 664,  11, 659,
  10, 654,  10, 649,  10, 644,   9, 640,   9, 635,   9, 630,   8, 625,   8, 620,
   8, 615,   7, 611,   7, 606,   7, 601,   6, 596,   6, 592,   6, 587,   6, 582,
   5, 577,   5, 573,   5, 568,   5, 563,   4, 559,   4, 554,   4, 550,   4, 545,
   4, 540,   3, 536,   3, 531,   3, 527,   3, 522,   3, 517,   2, 513,   2, 508,
   2, 504,   2, 499,   2, 495,   2, 491,   2, 486,   1, 482,   1, 477,   1, 473,
   1, 469,   1, 464,   1, 460,   1, 456,   1, 451,   1, 447,   1, 443,   1, 439,
   0, 434,   0, 430,   0, 426,   0, 422,   0, 418,   0, 414,   0, 410,   0, 405,
   0, 401,   0, 397,   0, 393,   0, 389,   0, 385,   0, 381,   0, 378,   0, 374,

}

func CLAMP16(i int) int {
	if i > math.MaxInt16 {
		return math.MaxInt16 
	}
	if i < math.MinInt16 {
		return math.MinInt16
	}
	return i
}

const (
		v_voll   = 0x00
 v_volr   = 0x01
		v_pitchl = 0x02
 v_pitchh = 0x03
		v_srcn   = 0x04
 v_adsr0  = 0x05
		v_adsr1  = 0x06
 v_gain   = 0x07
		v_envx   = 0x08
 v_outx   = 0x09
)

// 0: >>1  1: <<0  2: <<1 ... 12: <<11  13-15: >>4 <<11
var shifts =[16*2]int  {
						13,12,12,12,12,12,12,12,12,12,12, 12, 12, 16, 16, 16,
						 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 11, 11, 11,
}