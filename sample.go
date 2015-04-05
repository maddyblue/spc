package spc

func (s *SPC) Run(clock_count int) {
	REG := func(addr uint16) byte {
		return s.RAM[addr]
	}

	var new_phase int = s.phase + clock_count;
	var count int = new_phase >> 5;
	s.phase = new_phase & 31;
	if  count == 0 {
		return
	}
	
	dir := &s.RAM[REG(DIR) * 0x100]
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
			s.new_kon &= ~s.kon;
			s.kon    = s.new_kon;
			s.t_koff = REG(KOFF); 
		}
		
		s.run_counter( 1 );
		s.run_counter( 2 );
		s.run_counter( 3 );
		
		// Noise
		if  !READ_COUNTER( noise_rate ) 	{
			var feedback int = (s.noise << 13) ^ (s.noise << 14);
			s.noise = (feedback & 0x4000) ^ (s.noise >> 1);
		}
		
		// Voices
		var pmon_input int = 0;
		var main_out_l int = 0;
		var main_out_r int = 0;
		var echo_out_l int = 0;
		var echo_out_r int = 0;
		voice_t* v = s.voices;
		uint8_t* v_regs = s.regs;
		var vbit int = 1;
		for	{
			#define SAMPLE_PTR(i) GET_LE16A( &dir [VREG(v_regs,srcn) * 4 + i * 2] )
			
			var brr_header int = ram [v->brr_addr];
			var kon_delay int = v->kon_delay;
			
			// Pitch
			var pitch int = GET_LE16A( &VREG(v_regs,pitchl) ) & 0x3FFF;
			if  REG(PMON) & vbit 
				pitch += ((pmon_input >> 5) * pitch) >> 10;
			
			// KON phases
			if  --kon_delay >= 0 	{
				v->kon_delay = kon_delay;
				
				// Get ready to start BRR decoding on next sample
				if  kon_delay == 4 	{
					v->brr_addr   = SAMPLE_PTR( 0 );
					v->brr_offset = 1;
					v->buf_pos    = v->buf;
					brr_header    = 0; // header is ignored on this sample
				}
				
				// Envelope is never run during KON
				v->env        = 0;
				v->hidden_env = 0;
				
				// Disable BRR decoding until last three samples
				v->interp_pos = (kon_delay & 3 ? 0x4000 : 0);
				
				// Pitch is never added during KON
				pitch = 0;
			}
			
			var env int = v->env;
			
			// Gaussian interpolation
			{
				var output int = 0;
				VREG(v_regs,envx) = (uint8_t) (env >> 4);
				if  env 	{
					// Make pointers into gaussian based on fractional position between samples
					var offset int = (unsigned) v->interp_pos >> 3 & 0x1FE;
					short const* fwd = interleved_gauss       + offset;
					short const* rev = interleved_gauss + 510 - offset; // mirror left half of gaussian
					
					var const int* in = &v->buf_pos [(unsigned) v->interp_pos >> 12];
					
					if  !(slow_gaussian & vbit)  // 99%	{
						// Faster approximation when exact sample value isn't necessary for pitch mod
						output = (fwd [0] * in [0] +
						          fwd [1] * in [1] +
						          rev [1] * in [2] +
						          rev [0] * in [3]) >> 11;
						output = (output * env) >> 11;
					}	else	{
						output = (int16_t) (s.noise * 2);
						if  !(REG(NON) & vbit) 	{
							output  = (fwd [0] * in [0]) >> 11;
							output += (fwd [1] * in [1]) >> 11;
							output += (rev [1] * in [2]) >> 11;
							output = (int16_t) output;
							output += (rev [0] * in [3]) >> 11;
							
							CLAMP16( output );
							output &= ~1;
						}
						output = (output * env) >> 11 & ~1;
					}
					
					// Output
					var l int = output * v->volume [0];
					var r int = output * v->volume [1];
					
					main_out_l += l;
					main_out_r += r;
					
					if  REG(EON) & vbit 	{
						echo_out_l += l;
						echo_out_r += r;
					}
				}
				
				pmon_input = output;
				VREG(v_regs,outx) = (uint8_t) (output >> 8);
			}
			
			// Soft reset or end of sample
			if  REG(FLG) & 0x80 || (brr_header & 3) == 1 	{
				v->env_mode = env_release;
				env         = 0;
			}
			
			if  s.every_other_sample 	{
				// KOFF
				if  s.t_koff & vbit 
					v->env_mode = env_release;
				
				// KON
				if  s.kon & vbit 	{
					v->kon_delay = 5;
					v->env_mode  = env_attack;
					REG(ENDX) &= ~vbit;
				}
			}
			
			// Envelope
			if  !v->kon_delay 
			{
				// 97%
				if  v->env_mode == env_release  	{
					env -= 0x8;
					v->env = env;
					if  env <= 0 	{
						v->env = 0;
						goto skip_brr; // no BRR decoding for you!
					}
				// 3%
				}	else 	{
					var rate int;
					var const int adsr0 = VREG(v_regs,adsr0);
					var env_data int = VREG(v_regs,adsr1);
					// 97% ADSR
					if  adsr0 >= 0x80  	{
						if  v->env_mode > env_decay  // 89%	{
							env--;
							env -= env >> 8;
							rate = env_data & 0x1F;
							
							// optimized handling
							v->hidden_env = env;
							if  READ_COUNTER( rate ) 
								goto exit_env;
							v->env = env;
							goto exit_env;
						}	else if  v->env_mode == env_decay 	{
							env--;
							env -= env >> 8;
							rate = (adsr0 >> 3 & 0x0E) + 0x10;
						}	else // env_attack	{
							rate = (adsr0 & 0x0F) * 2 + 1;
							env += rate < 31 ? 0x20 : 0x400;
						}
					// GAIN
					}	else 	{
					
						var mode int;
						env_data = VREG(v_regs,gain);
						mode = env_data >> 5;
						if  mode < 4  // direct	{
							env = env_data * 0x10;
							rate = 31;
						}	else	{
							rate = env_data & 0x1F;
							if  mode == 4  // 4: linear decrease
							{
								env -= 0x20;
							}
							else if  mode < 6  // 5: exponential decrease
							{
								env--;
								env -= env >> 8;
							}
							else // 6,7: linear increase
							{
								env += 0x20;
								if  mode > 6 && (unsigned) v->hidden_env >= 0x600 
									env += 0x8 - 0x20; // 7: two-slope linear increase
							}
						}
					}
					
					// Sustain level
					if  (env >> 8) == (env_data >> 5) && v->env_mode == env_decay { 
						v->env_mode = env_sustain;
					}
					
					v->hidden_env = env;
					
					// unsigned cast because linear decrease going negative also triggers this
					if  (unsigned) env > 0x7FF 	{
						env = (env < 0 ? 0 : 0x7FF);
						if  v->env_mode == env_attack 
							v->env_mode = env_decay;
					}
					
					if  !READ_COUNTER( rate ) {
						v->env = env; // nothing else is controlled by the counter
					}
				}
			}
		exit_env:
			
			{
				// Apply pitch
				var old_pos int = v->interp_pos;
				var interp_pos int = (old_pos & 0x3FFF) + pitch;
				if  interp_pos > 0x7FFF {
					interp_pos = 0x7FFF;
				}
				v->interp_pos = interp_pos;
				
				// BRR decode if necessary
				if  old_pos >= 0x4000 	{
					// Arrange the four input nybbles in 0xABCD order for easy decoding
					var nybbles int = ram [(v->brr_addr + v->brr_offset) & 0xFFFF] * 0x100 +
							ram [(v->brr_addr + v->brr_offset + 1) & 0xFFFF];
					
					// Advance read position
					var const int brr_block_size = 9;
					var brr_offset int = v->brr_offset;
					if  (brr_offset += 2) >= brr_block_size 	{
						// Next BRR block
						var brr_addr int = (v->brr_addr + brr_block_size) & 0xFFFF;
						assert( brr_offset == brr_block_size );
						if  brr_header & 1 	{
							brr_addr = SAMPLE_PTR( 1 );
							if  !v->kon_delay {
								REG(ENDX) |= vbit;
							}
						}
						v->brr_addr = brr_addr;
						brr_offset  = 1;
					}
					v->brr_offset = brr_offset;
					
					// Decode
					
					// 0: >>1  1: <<0  2: <<1 ... 12: <<11  13-15: >>4 <<11
					static unsigned char const shifts [16 * 2] = {
						13,12,12,12,12,12,12,12,12,12,12, 12, 12, 16, 16, 16,
						 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 11, 11, 11
					};
					var const int scale = brr_header >> 4;
					var const int right_shift = shifts [scale];
					var const int left_shift  = shifts [scale + 16];
					
					// Write to next four samples in circular buffer
					int* pos = v->buf_pos;
					int* end;
					
					// Decode four samples
					for ( end = pos + 4; pos < end; pos++, nybbles <<= 4 )	{
						// Extract upper nybble and scale appropriately
						var s int = ((int16_t) nybbles >> right_shift) << left_shift;
						
						// Apply IIR filter (8 is the most commonly used)
						var const int filter = brr_header & 0x0C;
						var const int p1 = pos [brr_buf_size - 1];
						var const int p2 = pos [brr_buf_size - 2] >> 1;
						if  filter >= 8 	{
							s += p1;
							s -= p2;
							if  filter == 8  // s += p1 * 0.953125 - p2 * 0.46875	{
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
						CLAMP16( s );
						s = (int16_t) (s * 2);
						pos [brr_buf_size] = pos [0] = s; // second copy simplifies wrap-around
					}
					
					if  pos >= &v->buf [brr_buf_size] {
						pos = v->buf;
					}
					v->buf_pos = pos;
				}
			}
skip_brr:
			// Next voice
			vbit <<= 1;
			v_regs += 0x10;
			v++;
			if  vbit >= 0x100  {
				break;
			}
		}
		
		// Echo position
		var echo_offset int = s.echo_offset;
		uint8_t* const echo_ptr = &ram [(REG(ESA) * 0x100 + echo_offset) & 0xFFFF];
		if  !echo_offset {
			s.echo_length = (REG(EDL) & 0x0F) * 0x800;}
		echo_offset += 4;
		if  echo_offset >= s.echo_length {
			echo_offset = 0;}
		s.echo_offset = echo_offset;
		
		// FIR
		var echo_in_l int = GET_LE16SA( echo_ptr + 0 );
		var echo_in_r int = GET_LE16SA( echo_ptr + 2 );
		
		int (*echo_hist_pos) [2] = s.echo_hist_pos;
		if  ++echo_hist_pos >= &s.echo_hist [echo_hist_size] {
			echo_hist_pos = s.echo_hist;}
		s.echo_hist_pos = echo_hist_pos;
		
		echo_hist_pos [0] [0] = echo_hist_pos [8] [0] = echo_in_l;
		echo_hist_pos [0] [1] = echo_hist_pos [8] [1] = echo_in_r;
		
		#define CALC_FIR_( i, in )  ((in) * (int8_t) REG(fir + i * 0x10))
		echo_in_l = CALC_FIR_( 7, echo_in_l );
		echo_in_r = CALC_FIR_( 7, echo_in_r );
		
		#define CALC_FIR( i, ch )   CALC_FIR_( i, echo_hist_pos [i + 1] [ch] )
		#define DO_FIR( i )\
			echo_in_l += CALC_FIR( i, 0 );\
			echo_in_r += CALC_FIR( i, 1 );
		DO_FIR( 0 );
		DO_FIR( 1 );
		DO_FIR( 2 );
		#if defined (__MWERKS__) && __MWERKS__ < 0x3200
			__eieio(); // keeps compiler from stupidly "caching" things in memory
		#endif
		DO_FIR( 3 );
		DO_FIR( 4 );
		DO_FIR( 5 );
		DO_FIR( 6 );
		
		// Echo out
		if  !(REG(FLG) & 0x20) {
			var l int = (echo_out_l >> 7) + ((echo_in_l * (int8_t) REG(EFB)) >> 14);
			var r int = (echo_out_r >> 7) + ((echo_in_r * (int8_t) REG(EFB)) >> 14);
			
			// just to help pass more validation tests
			#if SPC_MORE_ACCURACY
				l &= ~1;
				r &= ~1;
			#endif
			
			CLAMP16( l );
			CLAMP16( r );
			
			SET_LE16A( echo_ptr + 0, l );
			SET_LE16A( echo_ptr + 2, r );
		}
		
		// Sound out
		var l int = (main_out_l * mvoll + echo_in_l * (int8_t) REG(EVOLL)) >> 14;
		var r int = (main_out_r * mvolr + echo_in_r * (int8_t) REG(EVOLR)) >> 14;
		
		CLAMP16( l );
		CLAMP16( r );
		
		if  (REG(FLG) & 0x40) 	{
			l = 0;
			r = 0;
		}
		
		sample_t* out = s.out;
		WRITE_SAMPLES( l, r, out );
		s.out = out;
		
		--count;
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

func (s *SPC) READ_COUNTER(rate byte) uint16 {
	return (*m.counter_select [rate] & s.counter_mask [rate])
}

var counter_mask [32]uint16 = {
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
	s.counters [2] = -0x20u;
	s.counters [3] =  0x0B;
	
	n := 2
	for i := 1; i < 32; i++ {
		s.counter_select [i] = &s.counters [n];
		--n
		if n == 0 {
			n = 3
		}
	}
	s.counter_select [ 0] = &s.counters [0]
	s.counter_select [30] = &s.counters [2]
}