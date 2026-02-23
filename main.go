package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	EthernetOltType = 0x88b6
	MaxOnu          = 128
)

var (
	Magic           = []byte{0xb9, 0x58, 0xd6, 0x3a}
	broadcastMAC, _ = net.ParseMAC("ff:ff:ff:ff:ff:ff")
)

type Cast struct {
	TX uint64 `json:"tx"`
	RX uint64 `json:"rx"`
}

type ONUStatus uint8

type OMCIMode uint16

// 54 50 4c 47 89 bd cf d8
type GPON_SN [8]byte

func (sn GPON_SN) IsValid() bool {
	if bytes.Equal(sn[:], (&GPON_SN{})[:]) {
		return false
	}
	return sn.Vendor() != nil && binary.BigEndian.Uint32(sn[4:]) > 0
}

func (sn GPON_SN) Vendor() *GPON_VENDOR {
	vendorSN := uint64(binary.BigEndian.Uint32(sn[:4]))
	for _, vendor := range GPONVendors {
		if vendorSN == vendor.Id {
			return &vendor
		}
	}
	return nil
}

func (sn GPON_SN) String() string {
	return fmt.Sprintf("%s%s", sn[:4], hex.EncodeToString(sn[4:]))
}

func (sn GPON_SN) MarshalText() ([]byte, error) {
	return []byte(sn.String()), nil
}

type ONU struct {
	Status  ONUStatus `json:"status"`
	ID      uint64    `json:"id"`
	TxPower float64   `json:"tx_power"`
	RxPower float64   `json:"rx_power"`
	Voltage uint64    `json:"voltage"`
	Current uint64    `json:"current"`
	Temp    uint64    `json:"temperature"`
}

type ONUOlt struct {
	OMCIMode  OMCIMode `json:"omci_mode"`
	OMCIErr   int      `json:"omci_err"`
	OnlineONU uint16   `json:"online_onu"`
	MaxONU    uint16   `json:"max_onu"`

	AuthLoid string `json:"auth_loid"`
	AuthPass string `json:"auth_passsword"`

	RogueONU uint `json:"rogue_onu"`

	ONUs map[GPON_SN]*ONU `json:"onu"`
}

type Olt struct {
	Up              time.Time        `json:"up"`
	MacAddr         net.HardwareAddr `json:"mac_addr"`
	FirmwareVersion string           `json:"fw_ver"`
	DNA             string           `json:"dna"`
	Temp            float64          `json:"temperature"`
	MaxTemp         float64          `json:"max_temperature"`
	Uplink          int64            `json:"uplink"`
	VCCINT          string           `json:"vccint"`
	VCCAUX          string           `json:"vccaux"`
	P2P             bool             `json:"p2p"`
	AN              bool             `json:"an"`
	CoverOffiline   bool             `json:"cover_offline"`
	Unicast         *Cast            `json:"unicast"`
	Broadcast       *Cast            `json:"broadcast"`
	Multicast       *Cast            `json:"multicast"`
	ONU             *ONUOlt          `json:"onu"`

	setupDone *atomic.Uint32
	log       *log.Logger `json:"-"`
	parent    *Olts       `json:"-"`
}

type Olts struct {
	pcapHandle  *pcap.Handle
	run         *atomic.Bool
	osSignalCtx context.Context

	macNetDev net.HardwareAddr
	newDev    chan struct{}
	log       *log.Logger

	olts *sync.Map
}

type Packet struct {
	Mac     net.HardwareAddr `json:"mac_addr"`
	Request uint16           `json:"request"`
	Type    uint16           `json:"id"`
	Status  uint8            `json:"status"`
	Reserv  uint8            `json:"reserv"`
	Check0  uint8            `json:"check0"`
	Check1  uint8            `json:"check1"`
	Header  []byte           `json:"header"`
	Data    []byte           `json:"data"`
}

func getPktData(mac net.HardwareAddr, raw []byte) (pkt *Packet, ok bool) {
	if len(raw) < 50 {
		return nil, false
	}

	if raw, ok = bytes.CutPrefix(raw, Magic); ok {
		// 0001 010c 01 00
		// 0 1  2 3  4  5
		pkt = &Packet{
			Data:    raw[8:],
			Header:  raw[:8],
			Request: binary.BigEndian.Uint16(raw[:2]),
			Type:    binary.BigEndian.Uint16(raw[2:4]),
			Status:  uint8(raw[4]),
			Reserv:  uint8(raw[5]),
			Check0:  uint8(raw[6]),
			Check1:  uint8(raw[7]),
			Mac:     mac,
		}
	}

	return
}

func (pkt Packet) String() string {
	return fmt.Sprintf("Request 0x%x - Type 0x%x (0x%x), Status 0x%x (%s)", pkt.Request, pkt.Type, pkt.Id(), pkt.Status, hex.EncodeToString(pkt.Header))
}

func (pkt Packet) Id() int {
	return (int(pkt.Request)+(int(pkt.Type)))>>(int(pkt.Reserv)>>8) + (int(pkt.Check0) | int(pkt.Check1))
}

func (pkt Packet) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"request": pkt.Request,
		"type":    pkt.Type,
		"status":  pkt.Status,
		"reserv":  pkt.Reserv,
		"check0":  pkt.Check0,
		"check1":  pkt.Check1,
		"mac":     pkt.Mac,
	})
}

func createPktData(request, reqType uint16, reserve, check0, check1 uint8, data []byte) []byte {
	newData := make([]byte, len(Magic), 50)
	copy(newData, Magic)
	newData = binary.BigEndian.AppendUint16(newData, request)
	newData = binary.BigEndian.AppendUint16(newData, reqType)
	newData = append(newData, reserve, check0, check1)
	if data != nil {
		copy(newData[len(newData):cap(newData)], data)
	}

	return newData[:cap(newData)]
}

func getInterfaceMAC(name string) net.HardwareAddr {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return net.HardwareAddr{0, 0, 0, 0, 0, 0}
	}
	return iface.HardwareAddr
}

const allLoad = 0x116+0x11a+0x2e+0x111+0x10f+0x119+0x118+0x117+0x20c

func (olt *Olt) ProcessPkt(pkt *Packet) error {
	if olt.setupDone.Load() <= allLoad {
		olt.setupDone.Add(uint32(pkt.Id()))
		olt.log.Printf("done %d", olt.setupDone.Load())

		switch pkt.Id() {
		case 0x116:
			u := float64(binary.BigEndian.Uint16(pkt.Data))/100
			olt.MaxTemp = u
			log.Printf("Max Temperature %f",  u)
		case 0x115:
			u := float64(binary.BigEndian.Uint16(pkt.Data))/100
			olt.Temp = u
			log.Printf("Temperature %f",  u)
		case 0x11a:
			// Check type return
			// byte(pkt.Data[0])
		case 0x2e:
			// ignore
		case 0x111:
			olt.FirmwareVersion = strings.TrimFunc(string(pkt.Data), func(r rune) bool {
				return unicode.IsSpace(r) || r == 0x0
			})
		case 0x10f:
			olt.ONU.MaxONU = uint16(pkt.Data[0])
			olt.log.Printf("Max ONU: %d", olt.ONU.MaxONU)
		case 0x119:
			olt.ONU.OnlineONU = uint16(pkt.Data[0])
			olt.log.Printf("Current ONU online: %d", olt.ONU.OnlineONU)
		case 0x118:
			olt.DNA = hex.EncodeToString(pkt.Data[:bytes.IndexByte(pkt.Data, 0x0)])
			olt.log.Printf("OLT DNA: %s", olt.DNA)
		case 0x117:
			now := time.Now()
			uptime := time.Duration(binary.BigEndian.Uint64(pkt.Data) * 16)
			olt.Up = now.Add(-uptime)
			olt.log.Printf("OLT Up time %s", olt.Up.String())
		default:
			olt.log.Printf("%s\n%s", pkt.String(), hex.Dump(pkt.Data))
		}

		if olt.setupDone.Load() >= allLoad {
			olt.SendPkt(0x24, 0x000c, 0x00, 0x02, 0x06, []byte{0x01})
		}

		return nil
	}

	olt.log.Println(pkt.String())
	switch pkt.Type {
	case 0x14:
		var sn GPON_SN
		copy(sn[:], pkt.Data)
		if !sn.IsValid() {
			olt.log.Printf("ONU %d update to offline", 0x1)
			return nil
		}

		if _, ok := olt.ONU.ONUs[sn]; !ok {
			olt.log.Printf("New ONU SN: %s", sn)
			olt.ONU.ONUs[sn] = &ONU{
				Status: 0,
			}
		}
		olt.SendPkt(0x25, 0xc, 0x00, 0x02, 0x10, []byte{0x01})
	case 0xd:
		olt.SendPkt(0x26, 0xc, 0x00, 0x02, 0x02, []byte{0x01})
	case 0x12:
		<-time.After(time.Second)
		lastUpdate := binary.BigEndian.Uint32(pkt.Data[2:6])
		olt.log.Println(olt.Up.Add(time.Duration(lastUpdate)).Format(time.RFC3339))

		olt.SendPkt(0x27, 0xc, 0x0, 0x2, 0x03, []byte{0x01})
		olt.SendPkt(0x28, 0xc, 0x0, 0x2, 0x0d, []byte{0x01})
	case 0x10c:
		olt.SendPkt(0x24, 0x000c, 0x00, 0x02, 0x06, []byte{0x01})
	}

	return nil
}

func (olt *Olt) Setup(pkt *Packet) error {
	olt.SendPkt(0x0002, 0x000c, 0x00, 0x00, 0x10, []byte{0xff})
	olt.SendPktBroadcast(0x0003, 0x000c, 0x00, 0x01, 0x18, []byte{0xff})
	olt.SendPktBroadcast(0x0005, 0x000c, 0x00, 0x00, 0x08, []byte{0xff})
	olt.SendPktBroadcast(0x0006, 0x000c, 0x00, 0x00, 0x01, []byte{0xff})
	olt.SendPktBroadcast(0x0007, 0x000c, 0x00, 0x00, 0x0f, []byte{0xff})
	olt.SendPktBroadcast(0x0008, 0x000c, 0x00, 0x00, 0x03, []byte{0xff})
	olt.SendPktBroadcast(0x0009, 0x000c, 0x00, 0x00, 0x04, []byte{0xff})
	olt.SendPktBroadcast(0x000a, 0x000c, 0x00, 0x01, 0x00, []byte{0xff})
	olt.SendPktBroadcast(0x000b, 0x000c, 0x00, 0x00, 0x05, []byte{0xff})
	olt.SendPktBroadcast(0x000c, 0x000c, 0x00, 0x00, 0x06, []byte{0xff})
	olt.SendPktBroadcast(0x000d, 0x000c, 0x00, 0x02, 0x00, []byte{0xff})
	olt.SendPktBroadcast(0x000e, 0x000c, 0x00, 0x01, 0x19, []byte{0xff})
	olt.SendPktBroadcast(0x0011, 0x000c, 0x00, 0x00, 0x02, []byte{0xff})
	olt.SendPktBroadcast(0x0012, 0x000c, 0x00, 0x00, 0x07, []byte{0xff})
	olt.SendPktBroadcast(0x0013, 0x000c, 0x00, 0x01, 0x0b, []byte{0xff})
	olt.SendPktBroadcast(0x0014, 0x000c, 0x00, 0x01, 0x0c, []byte{0xff})
	olt.SendPktBroadcast(0x0015, 0x000c, 0x00, 0x01, 0x0d, []byte{0xff})
	olt.SendPktBroadcast(0x0016, 0x000c, 0x00, 0x01, 0x09, []byte{0xff})
	olt.SendPktBroadcast(0x0017, 0x000c, 0x00, 0x01, 0x05, []byte{0xff})
	olt.SendPktBroadcast(0x0018, 0x000c, 0x00, 0x01, 0x07, []byte{0xff})
	olt.SendPktBroadcast(0x0019, 0x000c, 0x00, 0x01, 0x0a, []byte{0xff})
	olt.SendPktBroadcast(0x001a, 0x000c, 0x00, 0x01, 0x06, []byte{0xff})
	olt.SendPktBroadcast(0x001b, 0x000c, 0x00, 0x01, 0x08, []byte{0xff})
	olt.SendPktBroadcast(0x001c, 0x000c, 0x00, 0x01, 0x00, []byte{0xff})
	olt.SendPktBroadcast(0x001d, 0x000c, 0x00, 0x01, 0x11, []byte{0xff})
	olt.SendPktBroadcast(0x001e, 0x000c, 0x00, 0x01, 0x12, []byte{0xff})
	olt.SendPktBroadcast(0x001f, 0x000c, 0x00, 0x01, 0x13, []byte{0xff})
	olt.SendPktBroadcast(0x0020, 0x000c, 0x00, 0x02, 0x01, []byte{0x00})
	// olt.SendPkt(0x0022, 0x000c, 0x00, 0x02, 0x01, []byte{0x01})
	// olt.SendPkt(0x0023, 0x000c, 0x00, 0x02, 0x09, []byte{0x01})
	// olt.SendPkt(0x0024, 0x000c, 0x00, 0x02, 0x06, []byte{0x01})
	// olt.SendPkt(0x0025, 0x000c, 0x00, 0x02, 0x10, []byte{0x01})

	olt.SendPktBroadcast(0x0001, 0x000f, 0x00, 0x00, 0x09, []byte{0xff})
	olt.SendPkt(0x0008, 0x000c, 0x00, 0x00, 0x03, []byte{0xff})
	return nil
}

func (olt *Olt) SendPkt(request, reqType uint16, reserve, check0, check1 uint8, data []byte) error {
	return olt.SendPacket(createPktData(request, reqType, reserve, check0, check1, data))
}

func (olt *Olt) SendPktBroadcast(request, reqType uint16, reserve, check0, check1 uint8, data []byte) error {
	return olt.parent.SendPacket(broadcastMAC, createPktData(request, reqType, reserve, check0, check1, data))
}

func (olt *Olt) SendPacket(payload []byte) error {
	return olt.parent.SendPacket(olt.MacAddr, payload)
}

func (olts *Olts) SendPacket(dst net.HardwareAddr, payload []byte) error {
	eth := &layers.Ethernet{
		SrcMAC:       olts.macNetDev,
		DstMAC:       dst,
		EthernetType: layers.EthernetType(EthernetOltType),
	}

	buffer := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	gopacket.SerializeLayers(buffer, opts, eth, gopacket.Payload(payload))

	return olts.pcapHandle.WritePacketData(buffer.Bytes())
}

func (olts *Olts) getSourcePacket() iter.Seq2[*layers.Ethernet, error] {
	packetSource := gopacket.NewPacketSource(olts.pcapHandle, olts.pcapHandle.LinkType())

	return func(yield func(eth *layers.Ethernet, err error) bool) {
		for olts.run.Load() {
			pkt, err := packetSource.NextPacket()
			if err == nil {
				ethLayer := pkt.Layer(layers.LayerTypeEthernet)
				if ethLayer == nil {
					continue
				}
				if !yield(ethLayer.(*layers.Ethernet), nil) {
					return
				}
				continue
			}
		}
	}
}

func (olts *Olts) Close() error {
	olts.run.Store(false)
	olts.olts.Clear()
	return nil
}

func (olts *Olts) processPackets() {
	defer olts.Close()

	for eth, err := range olts.getSourcePacket() {
		if err != nil {
			return
		} else if eth.EthernetType != layers.EthernetType(EthernetOltType) {
			continue
		}

		if eth.SrcMAC.String() != olts.macNetDev.String() {
			if pkt, ok := getPktData(eth.SrcMAC, eth.Payload); ok {
				olt, ok := olts.olts.Load(pkt.Mac.String())
				if ok {
					go func() {
						if err = olt.(*Olt).ProcessPkt(pkt); err != nil {
							olts.log.Printf("error on process packet for %s, error: %s", pkt.Mac, err)
						}
					}()
					continue
				}

				newOlt := &Olt{
					parent:    olts,
					log:       log.New(os.Stdout, fmt.Sprintf("%s: ", pkt.Mac.String()), log.Ldate|log.Ltime|log.LUTC|log.Lmicroseconds|log.Lmsgprefix),
					setupDone: &atomic.Uint32{},

					MacAddr:   eth.SrcMAC,
					Unicast:   &Cast{},
					Broadcast: &Cast{},
					Multicast: &Cast{},
					Up:        time.Now(),
					ONU: &ONUOlt{
						ONUs: map[GPON_SN]*ONU{},
					},
				}

				olts.olts.Store(pkt.Mac.String(), newOlt)
				olts.log.Printf("[!] New OLT located: %s\n", pkt.Mac)
				if olts.newDev != nil {
					olts.newDev <- struct{}{}
				}

				if err = newOlt.Setup(pkt); err != nil {
					olts.log.Printf("error on process first packet for %s, error: %s", pkt.Mac, err)
					return
				}
			}
		}
	}
}

func main() {
	var olts Olts
	defer olts.Close()
	olts.run = &atomic.Bool{}
	olts.run.Store(true)
	olts.newDev = make(chan struct{})
	olts.olts = &sync.Map{}

	ifaceName := "eth0" // needs replace

	olts.macNetDev = getInterfaceMAC(ifaceName)
	olts.log = log.New(os.Stderr, fmt.Sprintf("%s %s: ", ifaceName, olts.macNetDev), log.Ldate|log.Ltime|log.LUTC|log.Lmicroseconds|log.Lmsgprefix)
	handle, err := pcap.OpenLive(ifaceName, 1600, true, pcap.BlockForever)
	if err != nil {
		olts.log.Fatalf("Error on open interface: %v", err)
	}
	defer handle.Close()
	olts.pcapHandle = handle
	go olts.processPackets()
	olts.log.Printf("Packet process started")

	// Stop process packages
	defer olts.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()
	olts.osSignalCtx = ctx

	// Send broadcast packet to get Mac and Firmware version
	discoveryData := createPktData(0x1, 0xc, 0x00, 0x00, 0x00, []byte{0xff})
	for range 2 {
		olts.SendPacket(broadcastMAC, discoveryData)
		<-time.After(time.Microsecond * 500)
	}

	firstTimeout := time.Second * 5
	select {
	case <-olts.newDev:
		close(olts.newDev)
	case <-time.After(firstTimeout):
		fmt.Fprintf(os.Stderr, "timeout on get first device (%s)\n", firstTimeout)
		stop()
		fmt.Printf("Stoping olts packets\n")
		olts.Close()
		fmt.Printf("Stoping pcap\n")
		handle.Close()
		fmt.Printf("Done\n")
		return
	}
	
	go http.ListenAndServe(":8081", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		olts.log.Printf("Request from %s", r.RequestURI)
		data := []*Olt{}
		
		olts.olts.Range(func(key, value any) bool {
			if olt, ok := value.(*Olt); ok {
				if olt.ONU != nil {
					data = append(data, olt)
				}
			}
			return true
		})
		
		w.Header().Set("Content-Type", "application/json; utf-8")
		// w.WriteHeader(200)
		js := json.NewEncoder(w)
		js.SetIndent("", "  ")
		if err = js.Encode(data); err != nil {
			olts.log.Printf("error on encode olt: %s\n", err)
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on encode olt data: %s\n", err)
		}
	}))

	<-ctx.Done()
	fmt.Fprint(os.Stderr, "\nexiting process!\n")
	os.Exit(0)
}
