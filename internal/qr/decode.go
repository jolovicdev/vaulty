package qr

import (
	"bytes"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"math"
	"strings"
)

// DecodeBytes decodes a QR code from PNG, JPEG or GIF image bytes.
func DecodeBytes(data []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrNotFound
	}
	return DecodeImage(img)
}

// DecodeImage locates and decodes a QR code in img. It handles standard
// screenshots, padded images, light or dark backgrounds, and 90-degree
// rotations.
func DecodeImage(img image.Image) (string, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 21 || h < 21 {
		return "", ErrNotFound
	}
	gray := toGray(img)
	thresh := otsuThreshold(gray)

	for _, invert := range []bool{false, true} {
		bin := binarize(gray, thresh, invert)
		if text, ok := decodeBinary(bin, w, h); ok {
			return text, nil
		}
	}
	return "", ErrNotFound
}

// DecodeMatrix decodes a QR symbol directly from its module matrix.
func DecodeMatrix(m Matrix) (string, error) {
	for rot := 0; rot < 4; rot++ {
		if text, err := decodeOriented(m); err == nil {
			return text, nil
		}
		m = rotateCW(m)
	}
	return "", ErrCorrupt
}

func decodeOriented(m Matrix) (string, error) {
	n := m.Size
	if n < 21 || (n-21)%4 != 0 {
		return "", ErrCorrupt
	}
	ver := (n-21)/4 + 1
	if ver < 1 || ver > 10 {
		return "", ErrCorrupt
	}

	ec, mask, ok := readFormatInfo(m)
	if !ok {
		return "", ErrCorrupt
	}

	_, fnMask := buildFunctionGrid(ver, n)
	unmasked := Matrix{Size: n, bits: append([]bool(nil), m.bits...)}
	applyMask(&unmasked, fnMask, mask)

	spec := versionSpecs[ver][ec]
	rawCW := extractCodewords(unmasked, fnMask, spec.totalCW())
	data, err := deinterleaveAndCorrect(rawCW, spec)
	if err != nil {
		return "", err
	}
	return parsePayload(data, ver)
}

func readFormatInfo(m Matrix) (ecLevel, int, bool) {
	n := m.Size
	c1x := []int{0, 1, 2, 3, 4, 5, 7, 8, 8, 8, 8, 8, 8, 8, 8}
	c1y := []int{8, 8, 8, 8, 8, 8, 8, 8, 7, 5, 4, 3, 2, 1, 0}
	var raw1 uint16
	for i := 0; i < 15; i++ {
		raw1 <<= 1
		if m.At(c1x[i], c1y[i]) {
			raw1 |= 1
		}
	}
	if ec, mask, ok := decodeFormat(raw1); ok {
		return ec, mask, true
	}

	var raw2 uint16
	for i := 0; i < 7; i++ {
		raw2 <<= 1
		if m.At(8, n-1-i) {
			raw2 |= 1
		}
	}
	for i := 0; i < 8; i++ {
		raw2 <<= 1
		if m.At(n-8+i, 8) {
			raw2 |= 1
		}
	}
	return decodeFormat(raw2)
}

func extractCodewords(m Matrix, fn []bool, total int) []byte {
	n := m.Size
	out := make([]byte, total)
	bitIdx := 0
	totalBits := total * 8
	up := true
	for x := n - 1; x > 0; x -= 2 {
		if x == 6 {
			x--
		}
		for i := 0; i < n; i++ {
			y := i
			if up {
				y = n - 1 - i
			}
			for dx := 0; dx < 2; dx++ {
				cx := x - dx
				if fn[y*n+cx] {
					continue
				}
				if bitIdx < totalBits && m.At(cx, y) {
					out[bitIdx/8] |= 1 << (7 - bitIdx%8)
				}
				bitIdx++
			}
		}
		up = !up
	}
	return out
}

func deinterleaveAndCorrect(cw []byte, spec ecSpec) ([]byte, error) {
	numBlocks := spec.g1Blocks + spec.g2Blocks
	blocks := make([][]byte, numBlocks)
	dataLens := make([]int, numBlocks)
	for i := 0; i < spec.g1Blocks; i++ {
		dataLens[i] = spec.g1Data
		blocks[i] = make([]byte, 0, spec.g1Data+spec.ecPerBlock)
	}
	for i := 0; i < spec.g2Blocks; i++ {
		idx := spec.g1Blocks + i
		dataLens[idx] = spec.g2Data
		blocks[idx] = make([]byte, 0, spec.g2Data+spec.ecPerBlock)
	}

	pos := 0
	maxData := max(spec.g1Data, spec.g2Data)
	for col := 0; col < maxData; col++ {
		for b := 0; b < numBlocks; b++ {
			if col < dataLens[b] {
				if pos >= len(cw) {
					return nil, ErrCorrupt
				}
				blocks[b] = append(blocks[b], cw[pos])
				pos++
			}
		}
	}
	for col := 0; col < spec.ecPerBlock; col++ {
		for b := 0; b < numBlocks; b++ {
			if pos >= len(cw) {
				return nil, ErrCorrupt
			}
			blocks[b] = append(blocks[b], cw[pos])
			pos++
		}
	}

	out := make([]byte, 0, spec.totalData())
	for _, blk := range blocks {
		fixed, err := rsDecode(blk, spec.ecPerBlock)
		if err != nil {
			return nil, err
		}
		out = append(out, fixed...)
	}
	return out, nil
}

const alnumTable = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

func digitChar(d int) byte {
	return alnumTable[d%10]
}

func parsePayload(data []byte, ver int) (string, error) {
	br := bitReader{buf: data}
	var out strings.Builder
	for br.remaining() >= 4 {
		mode, ok := br.read(4)
		if !ok || mode == 0 {
			break
		}
		switch mode {
		case 1: // numeric
			cntBits := 10
			if ver >= 10 {
				cntBits = 12
			}
			n, ok := br.read(cntBits)
			if !ok {
				return "", ErrCorrupt
			}
			for n >= 3 {
				v, ok := br.read(10)
				if !ok || v > 999 {
					return "", ErrCorrupt
				}
				out.WriteByte(digitChar(v / 100))
				out.WriteByte(digitChar((v / 10) % 10))
				out.WriteByte(digitChar(v % 10))
				n -= 3
			}
			switch n {
			case 2:
				v, ok := br.read(7)
				if !ok || v > 99 {
					return "", ErrCorrupt
				}
				out.WriteByte(digitChar(v / 10))
				out.WriteByte(digitChar(v % 10))
			case 1:
				v, ok := br.read(4)
				if !ok || v > 9 {
					return "", ErrCorrupt
				}
				out.WriteByte(digitChar(v))
			}
		case 2: // alphanumeric
			cntBits := 9
			if ver >= 10 {
				cntBits = 11
			}
			n, ok := br.read(cntBits)
			if !ok {
				return "", ErrCorrupt
			}
			for n >= 2 {
				v, ok := br.read(11)
				if !ok || v >= 45*45 {
					return "", ErrCorrupt
				}
				out.WriteByte(alnumTable[v/45])
				out.WriteByte(alnumTable[v%45])
				n -= 2
			}
			if n == 1 {
				v, ok := br.read(6)
				if !ok || v >= 45 {
					return "", ErrCorrupt
				}
				out.WriteByte(alnumTable[v])
			}
		case 4: // byte
			cntBits := 8
			if ver >= 10 {
				cntBits = 16
			}
			n, ok := br.read(cntBits)
			if !ok {
				return "", ErrCorrupt
			}
			for i := 0; i < n; i++ {
				v, ok := br.read(8)
				if !ok {
					return "", ErrCorrupt
				}
				out.WriteByte(byte(v & 0xff)) //nolint:gosec // read(8) returns 0..255
			}
		case 7: // ECI header: skip 1-3 bytes of assignment number.
			first, ok := br.read(8)
			if !ok {
				return "", ErrCorrupt
			}
			if first&0x80 != 0 {
				if first&0x40 == 0 {
					if _, ok := br.read(8); !ok {
						return "", ErrCorrupt
					}
				} else {
					if _, ok := br.read(16); !ok {
						return "", ErrCorrupt
					}
				}
			}
		default:
			return "", ErrCorrupt
		}
	}
	if out.Len() == 0 {
		return "", ErrCorrupt
	}
	return out.String(), nil
}

type bitReader struct {
	buf []byte
	pos int
}

func (r *bitReader) remaining() int { return len(r.buf)*8 - r.pos }

func (r *bitReader) read(n int) (int, bool) {
	if r.remaining() < n {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		v <<= 1
		if (r.buf[r.pos/8]>>(7-r.pos%8))&1 != 0 {
			v |= 1
		}
		r.pos++
	}
	return v, true
}

func rotateCW(m Matrix) Matrix {
	n := m.Size
	out := Matrix{Size: n, bits: make([]bool, n*n)}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			out.set(n-1-y, x, m.At(x, y))
		}
	}
	return out
}

type finderCenter struct {
	x, y   float64
	module float64
	count  int
}

func decodeBinary(bin []bool, w, h int) (string, bool) {
	centers := findFinders(bin, w, h)
	if len(centers) < 3 {
		return "", false
	}
	// Take the three strongest finder clusters and identify the right-angle
	// corner (top-left) plus top-right and bottom-left.
	for i := 0; i < len(centers) && i < 6; i++ {
		for j := i + 1; j < len(centers) && j < 6; j++ {
			for k := j + 1; k < len(centers) && k < 6; k++ {
				tl, tr, bl, ok := orientFinders(centers[i], centers[j], centers[k])
				if !ok {
					continue
				}
				if text, ok := sampleAndDecode(bin, w, h, tl, tr, bl); ok {
					return text, true
				}
			}
		}
	}
	return "", false
}

func findFinders(bin []bool, w, h int) []finderCenter {
	var clusters []finderCenter
	step := 1
	if h > 400 {
		step = 2
	}
	for y := 0; y < h; y += step {
		var runs [5]int
		state := 0
		rowOff := y * w
		for x := 0; x < w; x++ {
			dark := bin[rowOff+x]
			if dark {
				if state%2 == 1 {
					state++
				}
				runs[state]++
				continue
			}
			if state%2 == 0 {
				if state == 4 {
					if checkRatio(runs) {
						cx := float64(x) - float64(runs[4]+runs[3]) - float64(runs[2])/2.0
						mod := float64(runs[0]+runs[1]+runs[2]+runs[3]+runs[4]) / 7.0
						if cy, vMod, ok := checkVertical(bin, w, h, int(math.Round(cx)), y, mod); ok {
							addCluster(&clusters, cx, cy, (mod+vMod)/2)
						}
					}
					runs[0], runs[1], runs[2], runs[3], runs[4] = runs[2], runs[3], runs[4], 1, 0
					state = 3
				} else {
					state++
					runs[state]++
				}
			} else {
				runs[state]++
			}
		}
		if state == 4 && checkRatio(runs) {
			cx := float64(w) - float64(runs[4]+runs[3]) - float64(runs[2])/2.0
			mod := float64(runs[0]+runs[1]+runs[2]+runs[3]+runs[4]) / 7.0
			if cy, vMod, ok := checkVertical(bin, w, h, int(math.Round(cx)), y, mod); ok {
				addCluster(&clusters, cx, cy, (mod+vMod)/2)
			}
		}
	}

	// Sort clusters by confirmation count descending.
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0 && clusters[j].count > clusters[j-1].count; j-- {
			clusters[j], clusters[j-1] = clusters[j-1], clusters[j]
		}
	}
	return clusters
}

func checkRatio(r [5]int) bool {
	total := r[0] + r[1] + r[2] + r[3] + r[4]
	if total < 7 {
		return false
	}
	mod := float64(total) / 7.0
	tol := mod * 0.65
	return math.Abs(float64(r[0])-mod) < tol &&
		math.Abs(float64(r[1])-mod) < tol &&
		math.Abs(float64(r[2])-3*mod) < 3*tol &&
		math.Abs(float64(r[3])-mod) < tol &&
		math.Abs(float64(r[4])-mod) < tol
}

func checkVertical(bin []bool, w, h, x, y int, estMod float64) (float64, float64, bool) {
	if x < 0 || x >= w || !bin[y*w+x] {
		return 0, 0, false
	}
	var r [5]int
	i := y
	for i >= 0 && bin[i*w+x] {
		r[2]++
		i--
	}
	for i >= 0 && !bin[i*w+x] {
		r[1]++
		i--
	}
	for i >= 0 && bin[i*w+x] {
		r[0]++
		i--
	}
	if r[0] == 0 || r[1] == 0 {
		return 0, 0, false
	}
	j := y + 1
	for j < h && bin[j*w+x] {
		r[2]++
		j++
	}
	for j < h && !bin[j*w+x] {
		r[3]++
		j++
	}
	for j < h && bin[j*w+x] {
		r[4]++
		j++
	}
	if r[3] == 0 || r[4] == 0 || !checkRatio(r) {
		return 0, 0, false
	}
	vMod := float64(r[0]+r[1]+r[2]+r[3]+r[4]) / 7.0
	if math.Abs(vMod-estMod) > estMod*0.5 {
		return 0, 0, false
	}
	cy := float64(j) - float64(r[4]+r[3]) - float64(r[2])/2.0
	return cy, vMod, true
}

func addCluster(clusters *[]finderCenter, x, y, mod float64) {
	for i := range *clusters {
		c := &(*clusters)[i]
		if math.Abs(c.x-x) <= c.module*2 && math.Abs(c.y-y) <= c.module*2 {
			n := float64(c.count)
			c.x = (c.x*n + x) / (n + 1)
			c.y = (c.y*n + y) / (n + 1)
			c.module = (c.module*n + mod) / (n + 1)
			c.count++
			return
		}
	}
	*clusters = append(*clusters, finderCenter{x: x, y: y, module: mod, count: 1})
}

func dist(a, b finderCenter) float64 {
	return math.Hypot(a.x-b.x, a.y-b.y)
}

func orientFinders(a, b, c finderCenter) (tl, tr, bl finderCenter, ok bool) {
	dab, dbc, dca := dist(a, b), dist(b, c), dist(c, a)
	// The hypotenuse is the longest side; the opposite vertex is top-left.
	switch {
	case dbc >= dab && dbc >= dca:
		tl, tr, bl = a, b, c
	case dca >= dab && dca >= dbc:
		tl, tr, bl = b, a, c
	default:
		tl, tr, bl = c, a, b
	}
	d1, d2 := dist(tl, tr), dist(tl, bl)
	if d1 < 10 || d2 < 10 {
		return tl, tr, bl, false
	}
	if math.Abs(d1-d2) > max(d1, d2)*0.15 {
		return tl, tr, bl, false
	}
	// Ensure (tr - tl) x (bl - tl) > 0 (screen coords: x right, y down).
	cross := (tr.x-tl.x)*(bl.y-tl.y) - (tr.y-tl.y)*(bl.x-tl.x)
	if cross < 0 {
		tr, bl = bl, tr
	}
	return tl, tr, bl, true
}

func sampleAndDecode(bin []bool, w, h int, tl, tr, bl finderCenter) (string, bool) {
	avgMod := (tl.module + tr.module + bl.module) / 3.0
	if avgMod < 0.5 {
		return "", false
	}
	estDim := int(math.Round(((dist(tl, tr)+dist(tl, bl))/2.0)/avgMod)) + 7
	// Snap to valid QR dimensions (21 + 4*(v-1)) and try +-4 as fallback.
	snapped := ((estDim-21+2)/4)*4 + 21
	for _, dim := range []int{snapped, snapped - 4, snapped + 4} {
		if dim < 21 || dim > 57 {
			continue
		}
		m := sampleGrid(bin, w, h, tl, tr, bl, dim)
		if text, err := DecodeMatrix(m); err == nil {
			return text, true
		}
	}
	return "", false
}

func sampleGrid(bin []bool, w, h int, tl, tr, bl finderCenter, dim int) Matrix {
	span := float64(dim - 7)
	ux := (tr.x - tl.x) / span
	uy := (tr.y - tl.y) / span
	vx := (bl.x - tl.x) / span
	vy := (bl.y - tl.y) / span

	m := Matrix{Size: dim, bits: make([]bool, dim*dim)}
	for y := 0; y < dim; y++ {
		dy := float64(y) - 3.0
		for x := 0; x < dim; x++ {
			dx := float64(x) - 3.0
			px := int(math.Round(tl.x + dx*ux + dy*vx))
			py := int(math.Round(tl.y + dx*uy + dy*vy))
			if px >= 0 && py >= 0 && px < w && py < h {
				m.set(x, y, bin[py*w+px])
			}
		}
	}
	return m
}

func toGray(img image.Image) []uint8 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			lum := (299*(r>>8) + 587*(g>>8) + 114*(bl>>8)) / 1000
			out[y*w+x] = uint8(lum) //nolint:gosec // lum is in 0..255
		}
	}
	return out
}

func otsuThreshold(gray []uint8) uint8 {
	var hist [256]int
	for _, v := range gray {
		hist[v]++
	}
	total := float64(len(gray))
	var sum float64
	for i, c := range hist {
		sum += float64(i * c)
	}
	var sumB, wB, maxVar float64
	best := uint8(128)
	for t := 0; t < 256; t++ {
		wB += float64(hist[t])
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += float64(t * hist[t])
		mB := sumB / wB
		mF := (sum - sumB) / wF
		diff := mB - mF
		between := wB * wF * diff * diff
		if between > maxVar {
			maxVar = between
			best = uint8(t) //nolint:gosec // t is in 0..255
		}
	}
	return best
}

func binarize(gray []uint8, thresh uint8, invert bool) []bool {
	out := make([]bool, len(gray))
	for i, v := range gray {
		dark := v <= thresh
		if invert {
			dark = !dark
		}
		out[i] = dark
	}
	return out
}
