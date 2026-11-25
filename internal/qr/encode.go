package qr

import (
	"fmt"
	"math/bits"
	"strings"
)

// ecLevel matches the two-bit format indicator in ISO/IEC 18004 Table 12:
// L=1 (01), M=0 (00), Q=3 (11), H=2 (10).
type ecLevel int

const (
	ecM ecLevel = 0
	ecL ecLevel = 1
	ecH ecLevel = 2
	ecQ ecLevel = 3
)

type ecSpec struct {
	ecPerBlock int
	g1Blocks   int
	g1Data     int
	g2Blocks   int
	g2Data     int
}

func (s ecSpec) totalData() int {
	return s.g1Blocks*s.g1Data + s.g2Blocks*s.g2Data
}

func (s ecSpec) totalCW() int {
	return s.totalData() + (s.g1Blocks+s.g2Blocks)*s.ecPerBlock
}

// versionSpecs holds ISO/IEC 18004 Table 9 parameters for Versions 1..10,
// indexed by [version][ecLevel].
var versionSpecs = [11][4]ecSpec{
	{},
	// Version 1 (21x21, 26 CW)
	{
		ecM: {10, 1, 16, 0, 0},
		ecL: {7, 1, 19, 0, 0},
		ecH: {17, 1, 9, 0, 0},
		ecQ: {13, 1, 13, 0, 0},
	},
	// Version 2 (25x25, 44 CW)
	{
		ecM: {16, 1, 28, 0, 0},
		ecL: {10, 1, 34, 0, 0},
		ecH: {28, 1, 16, 0, 0},
		ecQ: {22, 1, 22, 0, 0},
	},
	// Version 3 (29x29, 70 CW)
	{
		ecM: {26, 1, 44, 0, 0},
		ecL: {15, 1, 55, 0, 0},
		ecH: {22, 2, 13, 0, 0},
		ecQ: {18, 2, 17, 0, 0},
	},
	// Version 4 (33x33, 100 CW)
	{
		ecM: {18, 2, 32, 0, 0},
		ecL: {20, 1, 80, 0, 0},
		ecH: {16, 4, 9, 0, 0},
		ecQ: {26, 2, 24, 0, 0},
	},
	// Version 5 (37x37, 134 CW)
	{
		ecM: {24, 2, 43, 0, 0},
		ecL: {26, 1, 108, 0, 0},
		ecH: {22, 2, 11, 2, 12},
		ecQ: {18, 2, 15, 2, 16},
	},
	// Version 6 (41x41, 172 CW)
	{
		ecM: {16, 4, 27, 0, 0},
		ecL: {18, 2, 68, 0, 0},
		ecH: {28, 4, 15, 0, 0},
		ecQ: {24, 4, 19, 0, 0},
	},
	// Version 7 (45x45, 196 CW)
	{
		ecM: {18, 4, 31, 0, 0},
		ecL: {20, 2, 78, 0, 0},
		ecH: {26, 4, 13, 1, 14},
		ecQ: {18, 2, 14, 4, 15},
	},
	// Version 8 (49x49, 242 CW)
	{
		ecM: {22, 2, 38, 2, 39},
		ecL: {24, 2, 97, 0, 0},
		ecH: {26, 4, 14, 2, 15},
		ecQ: {22, 4, 18, 2, 19},
	},
	// Version 9 (53x53, 292 CW)
	{
		ecM: {22, 3, 36, 2, 37},
		ecL: {30, 2, 116, 0, 0},
		ecH: {24, 4, 12, 4, 13},
		ecQ: {20, 4, 16, 4, 17},
	},
	// Version 10 (57x57, 346 CW)
	{
		ecM: {26, 4, 43, 1, 44},
		ecL: {18, 2, 68, 2, 69},
		ecH: {28, 6, 15, 2, 16},
		ecQ: {24, 6, 19, 2, 20},
	},
}

var alignCenters = [11][]int{
	nil,
	nil,
	{6, 18},
	{6, 22},
	{6, 26},
	{6, 30},
	{6, 34},
	{6, 22, 38},
	{6, 24, 42},
	{6, 26, 46},
	{6, 28, 50},
}

// Matrix is a square QR symbol where true means a dark module.
type Matrix struct {
	Size int
	bits []bool
}

// At reports whether module (x, y) is dark.
func (m Matrix) At(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Size || y >= m.Size {
		return false
	}
	return m.bits[y*m.Size+x]
}

func (m *Matrix) set(x, y int, dark bool) {
	m.bits[y*m.Size+x] = dark
}

// Encode builds a QR code matrix (EC level M, byte mode) for text.
func Encode(text string) (Matrix, error) {
	payload := []byte(text)
	ver := 0
	for v := 1; v <= 10; v++ {
		countBits := 8
		if v >= 10 {
			countBits = 16
		}
		needBytes := (4 + countBits + len(payload)*8 + 7) / 8
		if needBytes <= versionSpecs[v][ecM].totalData() {
			ver = v
			break
		}
	}
	if ver == 0 {
		return Matrix{}, ErrTooLong
	}

	spec := versionSpecs[ver][ecM]
	data := encodeByteData(payload, ver, spec.totalData())
	codewords := interleaveBlocks(data, spec)

	size := 21 + 4*(ver-1)
	base, fnMask := buildFunctionGrid(ver, size)
	placeData(&base, fnMask, codewords)

	bestMask := 0
	bestScore := -1
	var best Matrix
	for mask := 0; mask < 8; mask++ {
		cand := Matrix{Size: size, bits: append([]bool(nil), base.bits...)}
		applyMask(&cand, fnMask, mask)
		writeFormatInfo(&cand, ecM, mask)
		if ver >= 7 {
			writeVersionInfo(&cand, ver)
		}
		score := penalty(cand)
		if bestScore < 0 || score < bestScore {
			bestScore = score
			bestMask = mask
			best = cand
		}
	}
	_ = bestMask
	return best, nil
}

// EncodeSVG returns a crisp standalone SVG QR code with a 4-module quiet zone.
func EncodeSVG(text string) (string, error) {
	m, err := Encode(text)
	if err != nil {
		return "", err
	}
	const quiet = 4
	total := m.Size + 2*quiet
	var path strings.Builder
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if m.At(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#ffffff"/><path fill="#000000" d="%s"/></svg>`,
		total, total, path.String(),
	), nil
}

func encodeByteData(payload []byte, ver, dataCap int) []byte {
	var bw bitWriter
	bw.write(0x4, 4) // byte mode
	if ver >= 10 {
		bw.write(uint32(len(payload)), 16) //nolint:gosec // payload length bounded by V10 capacity
	} else {
		bw.write(uint32(len(payload)), 8) //nolint:gosec // payload length bounded by V9 capacity
	}
	for _, b := range payload {
		bw.write(uint32(b), 8)
	}
	// Up to 4 zero terminator bits, then pad to byte boundary.
	remBits := dataCap*8 - bw.len
	if remBits > 4 {
		remBits = 4
	}
	bw.write(0, remBits)
	if pad := (8 - bw.len%8) % 8; pad > 0 {
		bw.write(0, pad)
	}
	out := bw.bytes()
	for i := 0; len(out) < dataCap; i++ {
		if i%2 == 0 {
			out = append(out, 0xec)
		} else {
			out = append(out, 0x11)
		}
	}
	return out
}

func interleaveBlocks(data []byte, spec ecSpec) []byte {
	numBlocks := spec.g1Blocks + spec.g2Blocks
	dataBlocks := make([][]byte, 0, numBlocks)
	ecBlocks := make([][]byte, 0, numBlocks)
	off := 0
	for i := 0; i < spec.g1Blocks; i++ {
		blk := data[off : off+spec.g1Data]
		off += spec.g1Data
		dataBlocks = append(dataBlocks, blk)
		ecBlocks = append(ecBlocks, rsEncode(blk, spec.ecPerBlock))
	}
	for i := 0; i < spec.g2Blocks; i++ {
		blk := data[off : off+spec.g2Data]
		off += spec.g2Data
		dataBlocks = append(dataBlocks, blk)
		ecBlocks = append(ecBlocks, rsEncode(blk, spec.ecPerBlock))
	}

	out := make([]byte, 0, spec.totalCW())
	maxData := max(spec.g1Data, spec.g2Data)
	for col := 0; col < maxData; col++ {
		for _, blk := range dataBlocks {
			if col < len(blk) {
				out = append(out, blk[col])
			}
		}
	}
	for col := 0; col < spec.ecPerBlock; col++ {
		for _, blk := range ecBlocks {
			out = append(out, blk[col])
		}
	}
	return out
}

func markFunction(m *Matrix, fn []bool, x, y int, dark bool) {
	if x < 0 || y < 0 || x >= m.Size || y >= m.Size {
		return
	}
	m.set(x, y, dark)
	fn[y*m.Size+x] = true
}

func buildFunctionGrid(ver, size int) (Matrix, []bool) {
	m := Matrix{Size: size, bits: make([]bool, size*size)}
	fn := make([]bool, size*size)

	// Three 7x7 finder patterns plus 1-module white separators.
	for _, pos := range [][2]int{{0, 0}, {size - 7, 0}, {0, size - 7}} {
		ox, oy := pos[0], pos[1]
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				dark := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 &&
					(dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4))
				markFunction(&m, fn, ox+dx, oy+dy, dark)
			}
		}
	}

	// Timing patterns.
	for i := 8; i < size-8; i++ {
		markFunction(&m, fn, i, 6, i%2 == 0)
		markFunction(&m, fn, 6, i, i%2 == 0)
	}

	// Alignment patterns.
	centers := alignCenters[ver]
	for _, cy := range centers {
		for _, cx := range centers {
			if fn[cy*size+cx] {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					dark := dx == -2 || dx == 2 || dy == -2 || dy == 2 || (dx == 0 && dy == 0)
					markFunction(&m, fn, cx+dx, cy+dy, dark)
				}
			}
		}
	}

	// Reserve format info areas and dark module.
	for i := 0; i <= 8; i++ {
		if i != 6 {
			markFunction(&m, fn, i, 8, false)
			markFunction(&m, fn, 8, i, false)
		}
	}
	for i := 0; i < 8; i++ {
		markFunction(&m, fn, size-1-i, 8, false)
	}
	for i := 0; i < 7; i++ {
		markFunction(&m, fn, 8, size-1-i, false)
	}
	markFunction(&m, fn, 8, size-8, true)

	// Reserve version info areas for V7+.
	if ver >= 7 {
		for i := 0; i < 6; i++ {
			for j := 0; j < 3; j++ {
				markFunction(&m, fn, size-11+j, i, false)
				markFunction(&m, fn, i, size-11+j, false)
			}
		}
	}

	return m, fn
}

func placeData(m *Matrix, fn []bool, cw []byte) {
	size := m.Size
	bitIdx := 0
	totalBits := len(cw) * 8
	up := true
	for x := size - 1; x > 0; x -= 2 {
		if x == 6 {
			x--
		}
		for i := 0; i < size; i++ {
			y := i
			if up {
				y = size - 1 - i
			}
			for dx := 0; dx < 2; dx++ {
				cx := x - dx
				if fn[y*size+cx] {
					continue
				}
				dark := false
				if bitIdx < totalBits {
					dark = (cw[bitIdx/8]>>(7-bitIdx%8))&1 != 0
					bitIdx++
				}
				m.set(cx, y, dark)
			}
		}
		up = !up
	}
}

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	case 7:
		return ((x+y)%2+(x*y)%3)%2 == 0
	}
	return false
}

func applyMask(m *Matrix, fn []bool, mask int) {
	size := m.Size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !fn[y*size+x] && maskBit(mask, x, y) {
				m.set(x, y, !m.At(x, y))
			}
		}
	}
}

func formatBits(ec ecLevel, mask int) uint16 {
	data := (uint32(ec) << 3) | uint32(mask) //nolint:gosec // ec in 0..3, mask in 0..7
	rem := data << 10
	for i := 4; i >= 0; i-- {
		if rem&(1<<(i+10)) != 0 {
			rem ^= 0x537 << i
		}
	}
	return uint16(((data << 10) | rem) ^ 0x5412) //nolint:gosec // 15-bit format word
}

func writeFormatInfo(m *Matrix, ec ecLevel, mask int) {
	bits15 := formatBits(ec, mask)
	n := m.Size

	// Copy 1 around top-left finder (bits 14..0).
	c1x := []int{0, 1, 2, 3, 4, 5, 7, 8, 8, 8, 8, 8, 8, 8, 8}
	c1y := []int{8, 8, 8, 8, 8, 8, 8, 8, 7, 5, 4, 3, 2, 1, 0}
	for i := 0; i < 15; i++ {
		m.set(c1x[i], c1y[i], (bits15>>(14-i))&1 != 0)
	}

	// Copy 2 split along bottom-left (bits 14..8) and top-right (bits 7..0).
	for i := 0; i < 7; i++ {
		m.set(8, n-1-i, (bits15>>(14-i))&1 != 0)
	}
	for i := 0; i < 8; i++ {
		m.set(n-8+i, 8, (bits15>>(7-i))&1 != 0)
	}
}

func writeVersionInfo(m *Matrix, ver int) {
	rem := uint32(ver) << 12 //nolint:gosec // ver in 7..10
	for i := 5; i >= 0; i-- {
		if rem&(1<<(i+12)) != 0 {
			rem ^= 0x1f25 << i
		}
	}
	vbits := (uint32(ver) << 12) | rem //nolint:gosec // ver in 7..10
	n := m.Size
	for i := 0; i < 18; i++ {
		dark := (vbits>>i)&1 != 0
		a, b := i/3, n-11+i%3
		m.set(b, a, dark)
		m.set(a, b, dark)
	}
}

// penalty computes the standard ISO/IEC 18004 four-rule mask score.
func penalty(m Matrix) int {
	n := m.Size
	score := 0

	// Rule 1: 5+ same-colour runs horizontally and vertically.
	for y := 0; y < n; y++ {
		runH, runV := 1, 1
		for x := 1; x < n; x++ {
			if m.At(x, y) == m.At(x-1, y) {
				runH++
			} else {
				if runH >= 5 {
					score += 3 + (runH - 5)
				}
				runH = 1
			}
			if m.At(y, x) == m.At(y, x-1) {
				runV++
			} else {
				if runV >= 5 {
					score += 3 + (runV - 5)
				}
				runV = 1
			}
		}
		if runH >= 5 {
			score += 3 + (runH - 5)
		}
		if runV >= 5 {
			score += 3 + (runV - 5)
		}
	}

	// Rule 2: 2x2 solid blocks.
	for y := 0; y < n-1; y++ {
		for x := 0; x < n-1; x++ {
			c := m.At(x, y)
			if c == m.At(x+1, y) && c == m.At(x, y+1) && c == m.At(x+1, y+1) {
				score += 3
			}
		}
	}

	// Rule 3: finder-like 1:1:3:1:1 patterns with 4-module light margin.
	for y := 0; y < n; y++ {
		for x := 0; x <= n-11; x++ {
			if matchesFinderLikeRow(m, x, y) {
				score += 40
			}
			if matchesFinderLikeCol(m, y, x) {
				score += 40
			}
		}
	}

	// Rule 4: balance of dark modules vs 50%.
	dark := 0
	for _, b := range m.bits {
		if b {
			dark++
		}
	}
	pct := dark * 100 / (n * n)
	diff := pct - 50
	if diff < 0 {
		diff = -diff
	}
	score += (diff / 5) * 10
	return score
}

func matchesFinderLikeRow(m Matrix, x, y int) bool {
	var pat uint16
	for i := 0; i < 11; i++ {
		pat <<= 1
		if m.At(x+i, y) {
			pat |= 1
		}
	}
	return pat == 0b10111010000 || pat == 0b00001011101
}

func matchesFinderLikeCol(m Matrix, x, y int) bool {
	var pat uint16
	for i := 0; i < 11; i++ {
		pat <<= 1
		if m.At(x, y+i) {
			pat |= 1
		}
	}
	return pat == 0b10111010000 || pat == 0b00001011101
}

// decodeFormat finds the closest valid 15-bit format word within Hamming
// distance 3.
func decodeFormat(raw uint16) (ecLevel, int, bool) {
	bestDist := 4
	var bestEC ecLevel
	bestMask := 0
	for ec := ecLevel(0); ec < 4; ec++ {
		for mask := 0; mask < 8; mask++ {
			d := bits.OnesCount16(raw ^ formatBits(ec, mask))
			if d < bestDist {
				bestDist = d
				bestEC = ec
				bestMask = mask
			}
		}
	}
	return bestEC, bestMask, bestDist <= 3
}

type bitWriter struct {
	buf []byte
	len int
}

func (w *bitWriter) write(val uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.len%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if (val>>i)&1 != 0 {
			w.buf[w.len/8] |= 1 << (7 - w.len%8)
		}
		w.len++
	}
}

func (w *bitWriter) bytes() []byte { return w.buf }
