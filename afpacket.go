package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"

	"github.com/ghedo/go.pkt/routing"
)

var (
	device      string
	snapshotLen int32 = 100
	// err         error
	options gopacket.SerializeOptions
	// routermac   = "00:05:73:a0:00:00"
	block_size int = 8
)

type afpacketHandle struct {
	TPacket *afpacket.TPacket
}

func afpacketComputeSize(targetSizeMb int, snaplen int, pageSize int) (
	frameSize int, blockSize int, numBlocks int, err error) {

	if snaplen < pageSize {
		frameSize = pageSize / (pageSize / snaplen)
	} else {
		frameSize = (snaplen/pageSize + 1) * pageSize
	}

	// 128 is the default from the gopacket library so just use that
	blockSize = frameSize * 128
	numBlocks = (targetSizeMb * 1024 * 1024) / blockSize

	if numBlocks == 0 {
		return 0, 0, 0, fmt.Errorf("interface buffersize is too small")
	}

	return frameSize, blockSize, numBlocks, nil
}

func newAfpacketHandle(device string, snaplen int, block_size int, num_blocks int,
	useVLAN bool, timeout time.Duration) (*afpacketHandle, error) {

	h := &afpacketHandle{}
	var err error

	if device == "any" {
		h.TPacket, err = afpacket.NewTPacket(
			afpacket.OptFrameSize(snaplen),
			afpacket.OptBlockSize(block_size),
			afpacket.OptNumBlocks(num_blocks),
			afpacket.OptAddVLANHeader(useVLAN),
			afpacket.OptPollTimeout(timeout),
			afpacket.SocketRaw,
			afpacket.TPacketVersion3)
	} else {
		h.TPacket, err = afpacket.NewTPacket(
			afpacket.OptInterface(device),
			afpacket.OptFrameSize(snaplen),
			afpacket.OptBlockSize(block_size),
			afpacket.OptNumBlocks(num_blocks),
			afpacket.OptAddVLANHeader(useVLAN),
			afpacket.OptPollTimeout(timeout),
			afpacket.SocketRaw,
			afpacket.TPacketVersion3)
	}
	return h, err
}

// Close will close afpacket source.
func (h *afpacketHandle) Close() {
	h.TPacket.Close()
}

func workerafpacket(ch <-chan *net.IPAddr, dstAddr, dev string, routermac string) {
	szFrame, szBlock, numBlocks, err := afpacketComputeSize(block_size, int(snapshotLen), os.Getpagesize())
	if err != nil {
		log.Fatal(err)
	}

	dstIP := net.ParseIP(dstAddr)
	route, err := routing.RouteTo(dstIP)

	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	if dev != "" {
		device = dev
	} else if route == nil {
		log.Println("No route found")
	} else {
		device = route.Iface.Name
	}
	deviceInt, err := net.InterfaceByName(device)

	if err != nil {
		log.Fatal(err)
	}

	// Open device
	afpacketHandle, err := newAfpacketHandle(device, szFrame, szBlock, numBlocks, false, -time.Millisecond*10)
	if err != nil {
		log.Fatal(err)
	}
	defer afpacketHandle.Close()

	options = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: false, // true,
	}

	// rawBytes := []byte{10, 20, 30}

	// This time lets fill out some information
	srcIP, _ := getRouteSourceIP(route)
	ipLayer := &layers.IPv6{
		Version:    6,
		DstIP:      dstIP,
		SrcIP:      srcIP,
		NextHeader: layers.IPProtocolICMPv6,
		HopLimit:   64,
		// TypeCode:   layers.IPProtocolICMPv6,
	}

	dstmac, _ := net.ParseMAC(routermac)
	srcmac := deviceInt.HardwareAddr

	ethernetLayer := &layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv6,
		SrcMAC:       srcmac,
		DstMAC:       dstmac,
	}
	icmpLayer := &layers.ICMPv6{
		TypeCode: 128 << 8, //layers.ICMPv6TypeEchoRequest,
	}
	icmpLayer.SetNetworkLayerForChecksum(ipLayer)
	icmpEchoLayer := &layers.ICMPv6Echo{
		SeqNumber:  1,
		Identifier: 1,
	}

	// Create a properly formed packet, just with
	// empty details. Should fill out MAC addresses,
	// IP addresses, etc.
	// And create the packet with the layers
	buffer := gopacket.NewSerializeBuffer()
	for ip := range ch {
		ipLayer.DstIP = ip.IP
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			ipLayer,
			icmpLayer,
			icmpEchoLayer,
		)

		outgoingPacket := buffer.Bytes()
		_ = afpacketHandle.TPacket.WritePacketData(outgoingPacket)
	}
}

// Return the default IPv6 address of a network interface.
func getRouteSourceIP(r *routing.Route) (net.IP, error) {
	iface := r.Iface

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			ip6 := ipnet.IP.To16()
			if ip4 := ipnet.IP.To4(); ip4 == nil && ip6[0] != 0xfe {
				return ip6, nil
			}
		}
	}

	return nil, fmt.Errorf("no address found")
}
