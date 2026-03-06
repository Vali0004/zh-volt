package olt

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/gponsn"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

type ONUStatus uint8

const (
	ONUStatusOffline = ONUStatus(iota)
	ONUStatusOnline
	_
	ONUStatusDisconnected
	_
	_
	_
	ONUStatusOMCI
)

func (status ONUStatus) String() string {
	switch status {
	case ONUStatusOffline:
		return "Offline"
	case ONUStatusOnline:
		return "Online"
	case ONUStatusDisconnected:
		return "Disconnected"
	case ONUStatusOMCI:
		return "OMCI"
	}
	return fmt.Sprintf("Unknown (%d)", uint8(status))
}

func (status ONUStatus) MarshalText() ([]byte, error) {
	return []byte(status.String()), nil
}

type ONU struct {
	ID          uint8         `json:"id"`
	Status      ONUStatus     `json:"status"`
	Uptime      time.Duration `json:"uptime"`
	SN          gponsn.Sn     `json:"gpon_sn"`
	Voltage     uint64        `json:"voltage"`
	Current     uint64        `json:"current"`
	TxPower     float64       `json:"tx_power"`
	RxPower     float64       `json:"rx_power"`
	Temperature float32       `json:"temperature"`
	SetStatus   uint8         `json:"set_status"`

	Request map[string]any `json:"unmaped_requests"`

	Log *log.Logger `json:"-"`
}

type Olt struct {
	Uptime          time.Duration        `json:"uptime"`
	Mac             sources.HardwareAddr `json:"mac_addr"`
	FirmwareVersion string               `json:"fw_ver"`
	DNA             string               `json:"dna"`
	Temperature     float64              `json:"temperature"`
	MaxTemperature  float64              `json:"max_temperature"`
	OMCIMode        int                  `json:"omci_mode"`
	OMCIErr         int                  `json:"omci_err"`
	OnlineONU       uint8                `json:"online_onu"`
	MaxONU          uint8                `json:"max_onu"`
	ONUs            []*ONU               `json:"onu"`

	Log         *log.Logger
	parent      *OltManeger                               // Olt parent to send packets
	oltCallback *util.SyncMap[uint16, oltManegerCallback] // once callbacks
	onceStart   *sync.Once
}

func NewOlt(parent *OltManeger, macAddr sources.HardwareAddr) *Olt {
	return &Olt{
		parent:      parent,
		Mac:         macAddr,
		ONUs:        make([]*ONU, 0),
		Log:         log.New(parent.Log.Writer(), fmt.Sprintf("OLT %s: ", macAddr), defaultLogFlag),
		oltCallback: util.NewSyncMap[uint16, oltManegerCallback](),
		onceStart:   &sync.Once{},
	}
}

func (olt *Olt) sendPacket(raw *sources.PacketRaw) error {
	return olt.sendPacketCallback(raw, nil)
}

func (olt *Olt) sendPacketCallback(raw *sources.PacketRaw, call oltManegerCallback) error {
	if olt.parent.assignerRequestID(raw) {
		olt.oltCallback.Clear()
		// olt.parent.oltCallback
	}
	if call != nil {
		olt.oltCallback.Set(raw.Pkt.RequestID, call)
	}
	if olt.parent.Verbose > 3 {
		olt.Log.Printf("Seding pkt with ID %d", raw.Pkt.RequestID)
	}
	return olt.parent.pktSource.SendPkt(raw)
}

func (olt *Olt) sendPacketWait(raw *sources.PacketRaw, timeout ...time.Duration) (*sources.PacketRaw, error) {
	back := make(chan *sources.PacketRaw, 1)
	defer close(back)

	err := olt.sendPacketCallback(raw, func(pkt *sources.PacketRaw, olt *Olt, remove func()) {
		defer remove()
		if back != nil {
			select {
			case <-back:
			default:
				back <- pkt
			}
		}
	})

	if err != nil {
		return nil, err
	}

	tim := time.Second
	if len(timeout) > 0 {
		tim = timeout[0]
	}

	select {
	case pkt := <-back:
		return pkt, nil
	case <-time.After(tim):
		return nil, ErrCallbackTimeout
	}
}

func (olt *Olt) SetAuthLoid(loid string) error {
	info, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0d,
		Flag2:       0xff,
		Data:        []byte(loid),
	}}, time.Second*2)
	if err == nil && info.Pkt.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) SetAuthPass(pass string) error {
	info, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0c,
		Flag2:       0xff,
		Data:        []byte(pass),
	}}, time.Second*2)
	if err == nil && info.Pkt.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) OltInfo() {
	for {
		pkt, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get current temperature: %s", err)
			continue
		}
		olt.Temperature = (float64(binary.BigEndian.Uint16(pkt.Pkt.Data)) / 100) * 2

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x04, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get max temperature: %s", err)
			continue
		}
		olt.MaxTemperature = (float64(binary.BigEndian.Uint16(pkt.Pkt.Data)) / 100) * 2

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get current ONUs online: %s", err)
			continue
		}
		olt.OnlineONU = pkt.Pkt.Data[0]

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x01, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get OLT uptime: %s", err)
			continue
		}
		olt.Uptime = max(0, ((time.Duration(binary.BigEndian.Uint64(pkt.Pkt.Data)) * 16) - (time.Second * 2)).Round(time.Second))

		// Wait 5s to update info
		<-time.After(time.Second)
	}
}

func (olt *Olt) OnuUpdate() {
	olt.ONUs = make([]*ONU, olt.MaxONU)
	for onuID := range olt.MaxONU {
		onu := new(ONU)
		onu.ID = onuID
		onu.Request = make(map[string]any)
		onu.Log = log.New(olt.Log.Writer(), fmt.Sprintf("OLT %s, ONU %d: ", olt.Mac, onuID), olt.Log.Flags())
		olt.ONUs[onuID] = onu
		go olt.onu(onu)
	}
}

func (olt *Olt) onu(onu *ONU) {
	for {
		<-time.After(time.Second)
		olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x03, RequestType: 0x000c, Flag0: 0x2, Flag2: onu.ID}})
		pkt, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x0d, RequestType: 0x000c, Flag0: 0x2, Flag2: onu.ID}}, time.Second)
		if err == nil {
			onu.SetStatus = uint8(pkt.Pkt.Data[0])
		}

		// ONU Status
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x01, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second)
		if err != nil {
			onu.Log.Printf("return error on get status: %s", err)
			continue
		}
		onu.Status = ONUStatus(pkt.Pkt.Data[0])
		if onu.Status > ONUStatusOnline {
			onu.Uptime = 0
		}

		switch onu.Status {
		default:
			continue
		case ONUStatusDisconnected, ONUStatusOnline:
			// Get GPON SN
			pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x06, RequestType: 0x000c, Flag0: 0x02, Flag2: onu.ID}}, time.Second)
			if err != nil {
				onu.Log.Printf("error on get GPON SN: %s", err)
				continue
			} else if onu.SN, err = gponsn.Parse(pkt.Pkt.Data[:8]); err != nil {
				switch err {
				default:
					onu.Log.Printf("error on decode: %s", err)
					continue
				case gponsn.ErrSnNull:
					continue
				case gponsn.ErrSnInvalid, gponsn.ErrVendorInvalid:
					onu.Log.Printf("error on decode GPON SN")
					continue
				}
			}
		}

		// ONU Connection time
		if pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x02, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second); err == nil {
			onu.Uptime = max(0, (olt.Uptime - time.Duration(binary.BigEndian.Uint64(pkt.Pkt.Data)*16)).Round(time.Second))
		}

		if pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x0c, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second); err == nil {
			onu.Request[fmt.Sprintf("0x%02x", pkt.Pkt.Flag1)] = pkt.Pkt.Data
		}

		if pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x05, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second); err == nil {
			onu.Request[fmt.Sprintf("0x%02x", pkt.Pkt.Flag1)] = binary.BigEndian.Uint16(pkt.Pkt.Data[:2])
		}

		if pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x07, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second); err == nil {
			onu.Request[fmt.Sprintf("0x%02x", pkt.Pkt.Flag1)] = binary.BigEndian.Uint32(pkt.Pkt.Data[:4])
		}

		if pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{Flag1: 0x0f, Flag0: 0x02, RequestType: 0x000c, Flag2: onu.ID}}, time.Second); err == nil {
			onu.Request[fmt.Sprintf("0x%02x", pkt.Pkt.Flag1)] = pkt.Pkt.Data[:16]
		}
	}
}

func (olt *Olt) startPkt(pkt *sources.PacketRaw) {
	olt.onceStart.Do(func() {
		var err error
		olt.FirmwareVersion = strings.TrimFunc(string(pkt.Pkt.Data), func(r rune) bool {
			return unicode.IsSpace(r) || r == 0x0
		})

		// Get max ONUs
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x18, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get max ONUs: %s", err)
			return
		}
		olt.MaxONU = min(pkt.Pkt.Data[0], MaxONU)
		olt.Log.Printf("Max ONUs: %d", olt.MaxONU)

		// Get OLT DNA
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x08, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get OLT DNA: %s", err)
			return
		}
		olt.DNA = hex.EncodeToString(pkt.Pkt.Data[:bytes.IndexByte(pkt.Pkt.Data, 0x0)])
		olt.Log.Printf("OLT DNA: %s", olt.DNA)

		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x05, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x06, Flag2: 0xff}})

		// Get current ONUs connected
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get ONUs connected: %s", err)
			return
		}
		olt.OnlineONU = uint8(pkt.Pkt.Data[0])
		olt.Log.Printf("Current ONU online: %d", olt.OnlineONU)

		// Get current OMCI Mode
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x19, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get OMCI Mode: %s", err)
			return
		}
		olt.OMCIMode = int(byte(pkt.Pkt.Data[0]))
		olt.Log.Printf("OMCI Mode %d", olt.OMCIMode)

		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x02, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x07, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0b, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0c, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0d, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x09, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x05, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x07, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0a, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x06, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x08, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x11, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x12, Flag2: 0xff}})
		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x13, Flag2: 0xff}})

		// ??
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01, Flag2: 0x00}}, time.Second)
		if err != nil {
			olt.Log.Printf("??: %s", err)
			return
		}

		// OLT Config
		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000f, Flag1: 0x09, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("OLT Config error: %s", err)
			return
		}
		olt.Log.Printf("OLT Config %s", hex.EncodeToString(pkt.Pkt.Data))

		olt.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x10, Flag2: 0xff}})
		olt.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff}}, time.Second)

		go olt.OltInfo()
		go olt.OnuUpdate()
	})
}

func (olt *Olt) Packet(pkt *sources.PacketRaw) {
	if fn, ok := olt.oltCallback.Get(pkt.Pkt.RequestID); ok {
		olt.oltCallback.Del(pkt.Pkt.RequestID)
		fn(pkt, olt, func() {})
		return
	}

	olt.Log.Printf("PacketID %d droped process, %x", pkt.Pkt.RequestID, pkt.Pkt.Data)
}
