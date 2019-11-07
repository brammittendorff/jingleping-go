package main

import (
	"log"
	"net"
	//	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pfring"

	"github.com/ghedo/go.pkt/routing"
)

func workerPFRing(ch <-chan *net.IPAddr, dstAddr, dev string) {

	dstIP := net.ParseIP(dstAddr)
	route, err := routing.RouteTo(dstIP)

	var handle *pfring.Ring
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
	if handle, err = pfring.NewRing(device, 65536, 0); err != nil {
		log.Fatalln(err)
	}

	if err = handle.SetSocketMode(pfring.WriteOnly); err != nil {
		log.Fatalln(err)
	}

	if err = handle.Enable(); err != nil {
		log.Fatalln(err)
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
	for ip := range ch {
		ipLayer.DstIP = ip.IP
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			ipLayer,
			icmpLayer,
			icmpEchoLayer,
		)

		outgoingPacket := buffer.Bytes()
		err = handle.WritePacketData(outgoingPacket)
	}
}
