package main

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"net"
	"sort"
	"time"
)

type DrawingTechnique string

const (
	StandardTechnique DrawingTechnique = "standard"
	RandomTechnique   DrawingTechnique = "random"
	ScanlineTechnique DrawingTechnique = "scanline"
	SpiralTechnique   DrawingTechnique = "spiral"
	WaveTechnique     DrawingTechnique = "wave"
)

type Pixel struct {
	X, Y    int
	R, G, B uint32
	A       uint8
}

func collectPixels(img image.Image, bounds image.Rectangle) []Pixel {
	var pixels []Pixel
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			a8 := uint8(a >> 8)
			if a8 > 0 {
				pixels = append(pixels, Pixel{
					X: x,
					Y: y,
					R: r,
					G: g,
					B: b,
					A: a8,
				})
			}
		}
	}
	return pixels
}

func orderPixels(pixels []Pixel, technique DrawingTechnique, bounds image.Rectangle, waveAmp, waveFreq float64) []Pixel {
	switch technique {
	case RandomTechnique:
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(pixels), func(i, j int) {
			pixels[i], pixels[j] = pixels[j], pixels[i]
		})

	case SpiralTechnique:
		centerX := bounds.Dx() / 2
		centerY := bounds.Dy() / 2
		sort.Slice(pixels, func(i, j int) bool {
			x1, y1 := float64(pixels[i].X-centerX), float64(pixels[i].Y-centerY)
			x2, y2 := float64(pixels[j].X-centerX), float64(pixels[j].Y-centerY)
			angle1 := math.Atan2(y1, x1)
			angle2 := math.Atan2(y2, x2)
			dist1 := math.Sqrt(x1*x1 + y1*y1)
			dist2 := math.Sqrt(x2*x2 + y2*y2)
			turns1 := dist1/(2*math.Pi) + angle1/(2*math.Pi)
			turns2 := dist2/(2*math.Pi) + angle2/(2*math.Pi)
			return turns1 < turns2
		})

	case WaveTechnique:
		sort.Slice(pixels, func(i, j int) bool {
			wave1 := float64(pixels[i].Y) + waveAmp*math.Sin(waveFreq*float64(pixels[i].X))
			wave2 := float64(pixels[j].Y) + waveAmp*math.Sin(waveFreq*float64(pixels[j].X))
			return wave1 < wave2
		})
	}

	return pixels
}

func makeAddrsWithTechnique(img image.Image, dstNet string, xOff, yOff int, technique DrawingTechnique, waveAmp, waveFreq float64) []*net.IPAddr {
	bounds := img.Bounds()
	pixels := collectPixels(img, bounds)
	pixels = orderPixels(pixels, technique, bounds, waveAmp, waveFreq)

	var addrs []*net.IPAddr
	tip := net.ParseIP(fmt.Sprintf("%s::", dstNet))

	for _, p := range pixels {
		ip := make(net.IP, len(tip))
		copy(ip, tip)

		x := p.X + xOff
		y := p.Y + yOff

		ip[8] = byte(x >> 8)
		ip[9] = byte(x)
		ip[10] = byte(y >> 8)
		ip[11] = byte(y)
		ip[12] = byte(p.B >> 8)
		ip[13] = byte(p.G >> 8)
		ip[14] = byte(p.R >> 8)
		ip[15] = p.A

		addrs = append(addrs, &net.IPAddr{IP: ip})
	}

	return addrs
}
