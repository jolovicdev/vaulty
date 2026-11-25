package qr

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func renderImage(m Matrix, scale, margin int, invert bool) image.Image {
	size := (m.Size + 2*margin) * scale
	img := image.NewGray(image.Rect(0, 0, size, size))
	bg, fg := color.Gray{Y: 255}, color.Gray{Y: 0}
	if invert {
		bg, fg = fg, bg
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetGray(x, y, bg)
		}
	}
	for my := 0; my < m.Size; my++ {
		for mx := 0; mx < m.Size; mx++ {
			if !m.At(mx, my) {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetGray((mx+margin)*scale+dx, (my+margin)*scale+dy, fg)
				}
			}
		}
	}
	return img
}

func TestEncodeAndDecodeRoundTrip(t *testing.T) {
	payloads := []string{
		"otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP&issuer=GitHub",
		"otpauth://totp/Example%20Corp:alice@example.com?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&algorithm=SHA256&digits=8&period=60&issuer=Example%20Corp",
		"otpauth://totp/Short?secret=JBSWY3DP",
		"otpauth://totp/LongServiceName:" + strings.Repeat("a", 120) + "?secret=JBSWY3DPEHPK3PXP",
	}

	for _, want := range payloads {
		m, err := Encode(want)
		if err != nil {
			t.Fatalf("Encode(%q): %v", want, err)
		}
		got, err := DecodeMatrix(m)
		if err != nil {
			t.Fatalf("DecodeMatrix(%q): %v", want, err)
		}
		if got != want {
			t.Errorf("DecodeMatrix = %q, want %q", got, want)
		}

		// Verify PNG at multiple scales, JPEG compression, and dark-on-light vs light-on-dark.
		for _, tc := range []struct {
			name   string
			scale  int
			margin int
			invert bool
			asJPEG bool
		}{
			{"png 4x", 4, 4, false, false},
			{"png 8x wide margin", 8, 10, false, false},
			{"png inverted", 5, 4, true, false},
			{"jpeg 6x", 6, 6, false, true},
		} {
			img := renderImage(m, tc.scale, tc.margin, tc.invert)
			var buf bytes.Buffer
			if tc.asJPEG {
				if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := png.Encode(&buf, img); err != nil {
					t.Fatal(err)
				}
			}
			gotImg, err := DecodeBytes(buf.Bytes())
			if err != nil {
				t.Errorf("%s (%d bytes): DecodeBytes: %v", tc.name, len(want), err)
				continue
			}
			if gotImg != want {
				t.Errorf("%s: got %q, want %q", tc.name, gotImg, want)
			}
		}
	}
}

func TestReedSolomonCorrectsBitErrors(t *testing.T) {
	want := "otpauth://totp/Vaulty:test?secret=JBSWY3DPEHPK3PXP&issuer=Vaulty"
	m, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	// Flip several data modules inside the symbol; EC level M must recover them.
	damaged := Matrix{Size: m.Size, bits: append([]bool(nil), m.bits...)}
	for _, pt := range [][2]int{{12, 12}, {15, 14}, {18, 16}, {20, 18}} {
		damaged.set(pt[0], pt[1], !damaged.At(pt[0], pt[1]))
	}
	got, err := DecodeMatrix(damaged)
	if err != nil {
		t.Fatalf("DecodeMatrix(damaged): %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEncodeSVGProducesValidSVG(t *testing.T) {
	svg, err := EncodeSVG("otpauth://totp/GitHub:octocat?secret=JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(svg, "<svg ") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("unexpected SVG wrapper: %.40q", svg)
	}
	if !strings.Contains(svg, `shape-rendering="crispEdges"`) {
		t.Error("SVG missing crispEdges rendering hint")
	}
}

func TestDecodeRejectsBlankImage(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	if _, err := DecodeImage(img); err == nil {
		t.Error("DecodeImage succeeded on a blank image")
	}
}
