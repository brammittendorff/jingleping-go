package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

var (
	dstNetFlag  = flag.String("dst-net", "2001:610:1908:a000::", "the destination network of the ipv6 tree")
	imageFlag   = flag.String("image", "", "the image to ping to the tree")
	xOffFlag    = flag.Int("x", 0, "the x offset to draw the image")
	yOffFlag    = flag.Int("y", 0, "the y offset to draw the image")
	rateFlag    = flag.Int("rate", 5, "how many times to draw the image per second")
	workersFlag = flag.Int("workers", 1, "the number of workers to use")
	onceFlag    = flag.Bool("once", false, "abort after 1 loop")
	pcapFlag    = flag.Bool("pcap", false, "Use PCAP for sending")
	// pfringFlag        = flag.Bool("pfring", false, "Use PF_RING for sending")
	destInterfaceFlag = flag.String("interface", "eth0", "Use interface for outgoing traffic for pcap/pfring")
	routermac         = flag.String("destmac", "00:00:5e:00:01:77", "Destination mac interface ( aka router )")
	randomize         = flag.Bool("randomize", true, "randomize x/y for beter result on heavy pingload")

	techniqueFlag = flag.String("technique", "standard", "drawing technique (standard, random, scanline, spiral, wave)")
	waveAmpFlag   = flag.Float64("wave-amp", 10.0, "wave amplitude for wave drawing")
	waveFreqFlag  = flag.Float64("wave-freq", 0.1, "wave frequency for wave drawing")
)

const (
	maxX = 1920
	maxY = 1080
)

// filled on package initialization. Contains a simple ICMPv6 ECHO request.
var pingPacket []byte

// worker drains the incoming channel, sending ping packets to the incoming
// addresses.
func worker(ch <-chan *net.IPAddr) {
	log.Printf("starting worker")

	for {
		c, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
		if err != nil {
			log.Fatalf("could not open ping socket: %s", err)
		}

		for a := range ch {
			_, err = c.WriteTo(pingPacket, a)
			if err != nil {
				log.Printf("warning: could not send ping packet: %s", err)
				c2, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
				if err != nil {
					log.Fatalf("could not open ping socket: %s", err)
				} else {
					c.Close()
					c = c2
				}
			}
		}
	}
}

// fill fills the pixel channel with the frame(s) of desired image. Each frame
// has its own delay, which the filler uses to time consecutive frames. The
// filler loops forever.
func fill(ch chan<- *net.IPAddr, frames [][]*net.IPAddr, delay []time.Duration, rate int) {
	for {
	Frame:
		for fidx, frame := range frames {
			// frame clock
			ticker := time.NewTimer(delay[fidx])

			for {
				repeat := time.NewTimer(time.Second / time.Duration(rate))
				for _, a := range frame {
					ch <- a
				}
				if *onceFlag {
					for len(ch) != 0 {
						time.Sleep(1 * time.Second)
					}
					syscall.Kill(syscall.Getpid(), syscall.SIGINT)
					return
				}
				// then wait on both
				select {
				case <-ticker.C:
					continue Frame
				case <-repeat.C:
				}
			}
		}
	}
}

func shuffle(a []*net.IPAddr) {
	rand.Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
}

// makeAddrs takes an image or frame, along with the destination network of the
// display board and desired offset for the image, and yields a list of
// addresses to ping to draw the image to the board.
func makeAddrs(img image.Image, dstNet string, xOff, yOff int) []*net.IPAddr {
	var addrs []*net.IPAddr
	tip := net.ParseIP(dstNet)
	bounds := img.Bounds()
	for y := 0; y < bounds.Dy() && y+yOff <= maxY; y++ {
		if y+yOff < 0 {
			continue
		}
		for x := 0; x < bounds.Dx() && x+xOff <= maxX; x++ {
			if x+xOff < 0 {
				continue
			}
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			a = a >> 8
			if a > 0 {
				// Each channel is 16-bit, just shift down for 8-bit needed

				ip := make(net.IP, len(tip))
				copy(ip, tip)

				// x
				ip[8] = byte((x + xOff) >> 8)
				ip[9] = byte(x + xOff)
				// y
				ip[10] = byte((y + yOff) >> 8)
				ip[11] = byte(y + yOff)
				// rgba
				ip[12] = byte(b >> 8)
				ip[13] = byte(g >> 8)
				ip[14] = byte(r >> 8)
				ip[15] = uint8(a)

				addrs = append(addrs, &net.IPAddr{
					IP: ip,
				})
			}
		}
	}
	// os.Exit(0)
	if *randomize {
		shuffle(addrs)
	}
	return addrs
}

func main() {
	flag.Parse()

	if *imageFlag == "" {
		fmt.Fprintln(os.Stderr, "the image flag must be provided")
		os.Exit(1)
	}

	technique := DrawingTechnique(*techniqueFlag)

	// Validate technique
	switch technique {
	case StandardTechnique, RandomTechnique, ScanlineTechnique, SpiralTechnique, WaveTechnique:
		// valid
	default:
		log.Fatalf("invalid drawing technique: %s", technique)
	}

	var delays []time.Duration
	var frames [][]*net.IPAddr
	var qLen int

	{
		var imgs []image.Image

		{
			f, err := os.Open(*imageFlag)
			if err != nil {
				log.Fatalf("could not open image: %s", err)
			}
			defer f.Close()

			imgs, delays, err = decodeImage(f)
			if err != nil {
				log.Fatalf("could not decode image: %s", err)
			}
		}

		bounds := imgs[0].Bounds()
		log.Printf("image bounds: %d %d", bounds.Dx(), bounds.Dy())

		for _, img := range imgs {
			addrs := makeAddrsWithTechnique(
				img,
				*dstNetFlag,
				*xOffFlag,
				*yOffFlag,
				technique,
				*waveAmpFlag,
				*waveFreqFlag,
			)
			if len(addrs) > qLen {
				qLen = len(addrs)
			}
			frames = append(frames, addrs)
		}
	}

	// If delay isn't set at this point, we just have one image. Use the
	// provided flag to determine how many times to draw the image per second.
	if delays == nil {
		delays = []time.Duration{time.Second / time.Duration(*rateFlag)}
	}

	log.Printf("queue length: %d", qLen)

	pixCh := make(chan *net.IPAddr, qLen)
	go fill(pixCh, frames, delays, *rateFlag)

	for i := 0; i < *workersFlag; i++ {
		if *pcapFlag {
			go workerPCAP(pixCh, *dstNetFlag, *destInterfaceFlag, *routermac)
			// } else if(*pfringFlag) {
			// 	go workerPFRing(pixCh, *dstNetFlag)
		} else {
			go worker(pixCh)
		}
	}

	// wait for interruption
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	log.Println("exiting...")
}

// Setup ping packet
func init() {
	var err error

	p := &icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:  0xFFFF,
			Seq: 1,
		},
	}

	pingPacket, err = p.Marshal(nil)
	if err != nil {
		panic(err)
	}
}
