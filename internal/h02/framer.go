package h02

import "bytes"

// MaxTextFrame is the longest text frame accepted before the buffer is
// treated as garbage.
const MaxTextFrame = 1024

// Frame is one unit cut from the TCP byte stream.
type Frame struct {
	Text   string // a complete *...# text frame, or ""
	Binary []byte // bytes that were not a text frame (binary H02 or noise)
}

// Framer splits a TCP byte stream into frames. Text frames are cut on '*'
// and '#'. Anything else (for example the binary '$' format) is passed
// through as Binary so it can be logged as hex and studied later.
type Framer struct {
	buf []byte
}

// Push adds bytes read from the socket and returns any complete frames.
func (f *Framer) Push(b []byte) []Frame {
	f.buf = append(f.buf, b...)
	var out []Frame
	for len(f.buf) > 0 {
		if c := f.buf[0]; c == '\r' || c == '\n' || c == ' ' || c == 0 {
			f.take(1)
			continue
		}
		if f.buf[0] == '*' {
			end := bytes.IndexByte(f.buf, '#')
			if end < 0 {
				if len(f.buf) > MaxTextFrame {
					out = append(out, Frame{Binary: f.take(len(f.buf))})
				}
				break
			}
			out = append(out, Frame{Text: string(f.take(end + 1))})
			continue
		}
		// Binary H02: wait until the fixed part has arrived.
		if f.buf[0] == '$' && len(f.buf) < BinaryMinLen && !bytes.Contains(f.buf[1:], []byte("*HQ")) {
			break
		}
		// Not a text frame: pass through everything up to the next "*HQ".
		next := bytes.Index(f.buf[1:], []byte("*HQ"))
		if next < 0 {
			out = append(out, Frame{Binary: f.take(len(f.buf))})
			break
		}
		out = append(out, Frame{Binary: f.take(next + 1)})
	}
	return out
}

// Rest returns bytes held back waiting for the rest of a frame.
func (f *Framer) Rest() []byte {
	return f.buf
}

func (f *Framer) take(n int) []byte {
	c := make([]byte, n)
	copy(c, f.buf[:n])
	f.buf = f.buf[n:]
	return c
}
