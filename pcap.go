package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"github.com/ghedo/go.pkt/routing"
)

var (
	device      string
	snapshotLen int32 = 1024
	promiscuous       = false
	err         error
	options     gopacket.SerializeOptions
	router      = "00:05:73:a0:00:00"
)

func workerPCAP(ch <-chan *net.IPAddr, dstAddr, dev string) {
	timeout := 30 * time.Second

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

	// Open device
	handle, err := pcap.OpenLive(device, snapshotLen, promiscuous, timeout)
	if err != nil {
		log.Fatal(err)
	}
	defer handle.Close()

	options = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	// rawBytes := []byte{10, 20, 30}

	// Create a properly formed packet, just with
	// empty details. Should fill out MAC addresses,
	// IP addresses, etc.
	buffer := gopacket.NewSerializeBuffer()

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

	dstmac, _ := net.ParseMAC(router)
	ethernetLayer := &layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv6,
		SrcMAC:       route.Iface.HardwareAddr,
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

	// And create the packet with the layers
	buffer = gopacket.NewSerializeBuffer()

	gopacket.SerializeLayers(buffer, options,
		ethernetLayer,
		ipLayer,
		icmpLayer,
		icmpEchoLayer,
	)

	outgoingPacket := buffer.Bytes()

	for ip := range ch {
		rawip := ip.IP.To16()

		copy(outgoingPacket[38:], []byte(rawip))
		err = handle.WritePacketData(outgoingPacket)
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

	return nil, fmt.Errorf("No address found")
}
