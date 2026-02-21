// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Raw packet capture using gopacket/pcap

package capture

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"log"
)

type Capturer struct {
	iface  string
	handle *pcap.Handle
}

func NewCapturer(iface string) *Capturer {
	return &Capturer{
		iface: iface,
	}
}

func (c *Capturer) Start(ctx context.Context, packetChan chan<- []byte) error {
	if c.iface == "" {
		return fmt.Errorf("no interface specified")
	}
	// IPX EtherType is 0x8137. Also sometimes 0x8003 (older).
	// We use a BPF filter to capture only IPX packets.
	filter := "ether proto 0x8137"

	handle, err := pcap.OpenLive(c.iface, 1600, true, pcap.BlockForever)
	if err != nil {
		return fmt.Errorf("failed to open device %s: %v", c.iface, err)
	}
	c.handle = handle

	if err := handle.SetBPFFilter(filter); err != nil {
		log.Printf("Warning: failed to set BPF filter: %v", err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	go func() {
		defer handle.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case packet, ok := <-packetSource.Packets():
				if !ok {
					return
				}
				packetChan <- packet.Data()
			}
		}
	}()

	return nil
}

func (c *Capturer) Inject(data []byte) error {
	if c.handle == nil {
		return fmt.Errorf("capturer handle is nil")
	}
	return c.handle.WritePacketData(data)
}

func ListInterfaces() ([]string, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, d := range devices {
		names = append(names, d.Name)
	}
	return names, nil
}
