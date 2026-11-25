// Package qr implements offline ISO/IEC 18004 QR code encoding (Versions 1
// through 10, error correction level M) and decoding (Versions 1 through 10,
// all error correction levels L/M/Q/H) with zero external dependencies.
package qr

import "errors"

var (
	// ErrTooLong is returned when payload exceeds Version 10 capacity.
	ErrTooLong = errors.New("qr: payload exceeds maximum capacity")
	// ErrNotFound is returned when an image does not contain a readable QR code.
	ErrNotFound = errors.New("qr: no readable QR code found in image")
	// ErrCorrupt is returned when Reed-Solomon or payload decoding fails.
	ErrCorrupt = errors.New("qr: code is damaged or unsupported")
)

var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfInv(a byte) byte {
	return gfExp[255-int(gfLog[a])]
}

// rsGenerator returns the monic Reed-Solomon generator polynomial of degree n
// with roots alpha^0 .. alpha^(n-1), coefficients highest-degree first.
func rsGenerator(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(g)+1)
		root := gfExp[i]
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= gfMul(c, root)
		}
		g = next
	}
	return g
}

// rsEncode computes ecLen Reed-Solomon parity bytes for data.
func rsEncode(data []byte, ecLen int) []byte {
	gen := rsGenerator(ecLen)
	rem := make([]byte, ecLen)
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[ecLen-1] = 0
		if factor == 0 {
			continue
		}
		for j := 0; j < ecLen; j++ {
			rem[j] ^= gfMul(gen[j+1], factor)
		}
	}
	return rem
}

// rsDecode validates and, when needed, corrects up to ecLen/2 byte errors in
// cw (data followed by ecLen parity bytes), returning the corrected data slice.
func rsDecode(cw []byte, ecLen int) ([]byte, error) {
	if len(cw) <= ecLen {
		return nil, ErrCorrupt
	}
	buf := append([]byte(nil), cw...)
	synd := syndromes(buf, ecLen)
	if allZero(synd) {
		return buf[:len(buf)-ecLen], nil
	}

	// Berlekamp-Massey for error locator polynomial lambda(x) = 1 + l1*x + ...
	errLoc := []byte{1}
	oldLoc := []byte{1}
	for i := 0; i < ecLen; i++ {
		delta := synd[i]
		for j := 1; j < len(errLoc); j++ {
			delta ^= gfMul(errLoc[j], synd[i-j])
		}
		oldLoc = append([]byte{0}, oldLoc...)
		if delta == 0 {
			continue
		}
		if len(oldLoc) > len(errLoc) {
			next := polyScale(oldLoc, delta)
			oldLoc = polyScale(errLoc, gfInv(delta))
			errLoc = next
		}
		errLoc = polyAdd(errLoc, polyScale(oldLoc, delta))
	}

	numErr := len(errLoc) - 1
	if numErr == 0 || numErr*2 > ecLen {
		return nil, ErrCorrupt
	}

	// Chien search: find indices p where lambda(alpha^-p) == 0.
	n := len(buf)
	errPos := make([]int, 0, numErr)
	for p := 0; p < n; p++ {
		invX := gfExp[(255-(p%255))%255]
		if polyEvalLow(errLoc, invX) == 0 {
			errPos = append(errPos, p)
		}
	}
	if len(errPos) != numErr {
		return nil, ErrCorrupt
	}

	// Error evaluator polynomial omega(x) = synd(x) * lambda(x) mod x^ecLen.
	omega := make([]byte, ecLen)
	for i := 0; i < ecLen; i++ {
		for j := 0; j < len(errLoc) && i+j < ecLen; j++ {
			omega[i+j] ^= gfMul(synd[i], errLoc[j])
		}
	}

	// Formal derivative of lambda(x) in GF(2^8): keep odd-power terms shifted down.
	deriv := make([]byte, len(errLoc)-1)
	for i := 1; i < len(errLoc); i += 2 {
		deriv[i-1] = errLoc[i]
	}

	// Forney algorithm: magnitude = X * omega(X^-1) / lambda'(X^-1) where X = alpha^p.
	for _, p := range errPos {
		x := gfExp[p%255]
		invX := gfExp[(255-(p%255))%255]
		num := gfMul(x, polyEvalLow(omega, invX))
		den := polyEvalLow(deriv, invX)
		if den == 0 {
			return nil, ErrCorrupt
		}
		buf[n-1-p] ^= gfMul(num, gfInv(den))
	}

	if !allZero(syndromes(buf, ecLen)) {
		return nil, ErrCorrupt
	}
	return buf[:n-ecLen], nil
}

func syndromes(cw []byte, ecLen int) []byte {
	s := make([]byte, ecLen)
	for i := 0; i < ecLen; i++ {
		var acc byte
		root := gfExp[i]
		for _, b := range cw {
			acc = gfMul(acc, root) ^ b
		}
		s[i] = acc
	}
	return s
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func polyEvalLow(p []byte, x byte) byte {
	var acc byte
	for i := len(p) - 1; i >= 0; i-- {
		acc = gfMul(acc, x) ^ p[i]
	}
	return acc
}

func polyScale(p []byte, k byte) []byte {
	out := make([]byte, len(p))
	for i, v := range p {
		out[i] = gfMul(v, k)
	}
	return out
}

func polyAdd(a, b []byte) []byte {
	n := max(len(a), len(b))
	out := make([]byte, n)
	copy(out, a)
	for i, v := range b {
		out[i] ^= v
	}
	return out
}
