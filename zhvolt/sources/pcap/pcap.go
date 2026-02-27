package pcap

import (
	"fmt"
	"iter"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/packet"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/sources"
)

const (
	EthernetOltType = 0x88b6
	Timeout         = time.Microsecond * 5
)

var (
	_ sources.Sources = &Pcap{}

	optsSerealize = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
)

type Pcap struct {
	macNetDev net.HardwareAddr
	pkts      chan gopacket.Packet

	pcapHandle   *pcap.Handle
	packetSource *gopacket.PacketSource
}

func getInterfaceMAC(name string) net.HardwareAddr {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return net.HardwareAddr{0, 0, 0, 0, 0, 0}
	}
	return iface.HardwareAddr
}

func New(ifaceName string) (sources.Sources, error) {
	macNetDev := getInterfaceMAC(ifaceName)
	handle, err := pcap.OpenLive(ifaceName, 1600, true, Timeout)
	if err != nil {
		return nil, fmt.Errorf("error on open interface: %v", err)
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	return &Pcap{
		macNetDev: macNetDev,
		pkts:      packetSource.Packets(),

		pcapHandle:   handle,
		packetSource: packetSource,
	}, nil
}

func (pcap *Pcap) Close() error {
	if pcap.pcapHandle != nil {
		pcap.pcapHandle.Close()
	}
	return nil
}

func (pcap Pcap) MacAddr() net.HardwareAddr {
	return pcap.macNetDev
}

func (pcap *Pcap) GetPacketData() iter.Seq2[*sources.PacketRaw, error] {
	return func(yield func(*sources.PacketRaw, error) bool) {
		defer pcap.Close()
		for pkt := range pcap.pkts {
			ethLayer := pkt.Layer(layers.LayerTypeEthernet)
			if ethLayer == nil {
				continue
			}

			eth := ethLayer.(*layers.Ethernet)
			if eth.SrcMAC.String() == pcap.macNetDev.String() {
				continue
			}

			if eth.EthernetType != layers.EthernetType(EthernetOltType) || !packet.IsOltPacket(eth.Payload) {
				continue
			}

			if !yield(&sources.PacketRaw{Data: eth.Payload, MacSrc: eth.SrcMAC}, nil) {
				return
			}
		}
	}
}

func (pcap *Pcap) SendPacketData(dst net.HardwareAddr, data []byte) error {
	eth := &layers.Ethernet{
		SrcMAC:       pcap.macNetDev,
		DstMAC:       dst,
		EthernetType: layers.EthernetType(EthernetOltType),
	}

	buffer := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buffer, optsSerealize, eth, gopacket.Payload(data))
	return pcap.pcapHandle.WritePacketData(buffer.Bytes())
}
