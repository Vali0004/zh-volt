package zhvolt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/packet"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/sources"
)

var broadcastMAC = packet.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

const (
	MaxOnu = 128

	defaultLogFlag = log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds | log.Lmsgprefix
)

// Run callbacks on return response
type RequestCallback func(pkt packet.Packet, olt *Olt)

type Maneger struct {
	CorrentRequest uint16
	Log            *log.Logger

	run     *atomic.Bool
	context context.Context
	source  sources.Sources
	newDev  chan struct{}

	oltsSync *sync.RWMutex
	olts     map[string]*Olt

	reqCallbacks, initRequest *sync.Map
}

func (maneger Maneger) GetOlts() (olts map[string]*Olt) {
	return maneger.olts
}

func (maneger *Maneger) nextRequestID() (id uint16, overflow bool) {
	if (^uint16(0)) == maneger.CorrentRequest {
		overflow = true
	}
	maneger.CorrentRequest++
	id = maneger.CorrentRequest
	return
}

func (maneger *Maneger) SendRequestPacketCall(pkt packet.Packet, init bool, fn RequestCallback) error {
	var cleanRequests bool
	if pkt.RequestID, cleanRequests = maneger.nextRequestID(); cleanRequests {
		maneger.initRequest.Clear()
	}

	maneger.reqCallbacks.Store(pkt.RequestID, fn)
	if init {
		maneger.reqCallbacks.Delete(pkt.RequestID)
		maneger.initRequest.Store(pkt.RequestID, fn)
	}

	data, err := pkt.MarshalBinary()
	if err != nil {
		return err
	}
	return maneger.source.SendPacketData(pkt.Mac.Net(), data)
}

// Send Packet (this replace pkt.Request id)
func (maneger *Maneger) SendRequestPacket(pkt packet.Packet) error {
	return maneger.SendRequestPacketCall(pkt, false, nil)
}

func (maneger *Maneger) SendPktBroadcast(pkt packet.Packet) error {
	pkt.Mac = broadcastMAC
	return maneger.SendRequestPacket(pkt)
}

func (maneger *Maneger) SendPktBroadcastFn(pkt packet.Packet, fn RequestCallback) error {
	pkt.Mac = broadcastMAC
	return maneger.SendRequestPacketCall(pkt, true, fn)
}

func (maneger *Maneger) Close() error {
	maneger.run.Store(false)
	maneger.olts = nil
	return maneger.source.Close()
}

func (maneger *Maneger) saveOlt(mac packet.HardwareAddr, olt *Olt) {
	maneger.oltsSync.Lock()
	defer maneger.oltsSync.Unlock()
	if maneger.olts == nil {
		maneger.olts = make(map[string]*Olt)
	}
	maneger.olts[mac.String()] = olt
}

func (maneger *Maneger) getOlt(mac packet.HardwareAddr) (*Olt, bool) {
	maneger.oltsSync.RLock()
	defer maneger.oltsSync.RUnlock()

	olt, ok := maneger.olts[mac.String()]
	if ok {
		return olt, ok
	}
	return nil, ok
}

func (maneger *Maneger) processPackets() {
	defer maneger.Close()
	for sourceData, err := range maneger.source.GetPacketData() {
		if err != nil {
			return
		}

		var pkt packet.Packet
		err := pkt.Unmarshal(packet.HardwareAddr(sourceData.MacSrc), sourceData.Data)
		if err != nil && (err == packet.ErrNoMagic || err == packet.ErrNotValid) {
			continue
		} else if err != nil {
			maneger.Log.Printf("%s: error on process pkt: %s", sourceData.MacSrc, err)
			return
		}

		olt, ok := maneger.getOlt(pkt.Mac)
		if !ok {
			olt = &Olt{
				parent: maneger,
				Log:    log.New(maneger.Log.Writer(), fmt.Sprintf("ONU %s: ", pkt.Mac), defaultLogFlag),

				MacAddr: pkt.Mac,
				ONUs:    map[uint8]*ONU{},
			}

			maneger.saveOlt(pkt.Mac, olt)
			maneger.Log.Printf("New OLT: %s\n", pkt.Mac)
			if maneger.newDev != nil {
				maneger.newDev <- struct{}{}
			}
		}

		// Process packet and delete from map
		if callback, ok := maneger.initRequest.Load(pkt.RequestID); ok {
			if fn, ok := callback.(RequestCallback); ok && fn != nil {
				go fn(pkt, olt)
				continue
			}
		}

		// Process packet and delete from map
		if callback, ok := maneger.reqCallbacks.LoadAndDelete(pkt.RequestID); ok {
			if fn, ok := callback.(RequestCallback); ok && fn != nil {
				go fn(pkt, olt)
				continue
			}
		}

		go olt.ProcessPkt(pkt)
	}
}

// Return new Olt Maneger process
func NewOltProcess(source sources.Sources, logWrite io.Writer, ctx context.Context) (*Maneger, error) {
	maneger := &Maneger{
		run:          &atomic.Bool{},
		newDev:       make(chan struct{}),
		oltsSync:     &sync.RWMutex{},
		reqCallbacks: &sync.Map{},
		initRequest:  &sync.Map{},
		source:       source,
		context:      ctx,
	}

	maneger.run.Store(true)
	if logWrite == nil {
		logWrite = io.Discard
	}
	maneger.Log = log.New(logWrite, fmt.Sprintf("OLT %s: ", source.MacAddr()), defaultLogFlag)
	go maneger.processPackets()

	// Send broadcast packet
	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.FirmwareVersion = strings.TrimFunc(string(pkt.Data), func(r rune) bool {
			return unicode.IsSpace(r) || r == 0x0
		})

	})

	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x18, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.MaxONU = uint8(pkt.Data[0])
		olt.Log.Printf("Max ONU: %d", olt.MaxONU)
	})

	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag1: 0x08, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.DNA = hex.EncodeToString(pkt.Data[:bytes.IndexByte(pkt.Data, 0x0)])
		olt.Log.Printf("OLT DNA: %s", olt.DNA)
	})

	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag1: 0x01, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		now := time.Now()
		uptime := time.Duration(binary.BigEndian.Uint64(pkt.Data) * 16)
		olt.Up = now.Add(-uptime)
		olt.Log.Printf("OLT Up time %s", olt.Up.Format(time.RFC3339Nano))
	})

	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag1: 0x05, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag1: 0x06, Flag2: 0xff})
	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x02, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.OnlineONU = uint8(pkt.Data[0])
		olt.Log.Printf("Current ONU online: %d", olt.OnlineONU)
	})

	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x19, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.OMCIMode = int(byte(pkt.Data[0]))
		olt.Log.Printf("OMCI Mode %d", olt.OMCIMode)
	})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag1: 0x02, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag1: 0x07, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x0b, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x0c, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x0d, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x09, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x05, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x07, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x0a, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x06, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x08, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x11, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x12, Flag2: 0xff})
	maneger.SendPktBroadcast(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x13, Flag2: 0xff})
	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x02, Flag1: 0x01, Flag2: 0x00}, func(pkt packet.Packet, olt *Olt) {})
	maneger.SendPktBroadcastFn(packet.Packet{Type: 0x000f, Flag1: 0x09, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.Log.Printf("OLT Data: %s", hex.EncodeToString(pkt.Data))
		olt.Onu()
	})

	firstTimeout := time.Second * 5
	select {
	case <-maneger.newDev:
		close(maneger.newDev)
	case <-time.After(firstTimeout):
		return nil, fmt.Errorf("timeout on get first device (%s)\n", firstTimeout)
	}

	return maneger, nil
}
