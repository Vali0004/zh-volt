package olt

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/gponsn"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
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

	Log *slog.Logger `json:"-"`
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

	Log       *slog.Logger
	parent    *OltManeger // Olt parent to send packets
	onceStart *sync.Once
}

func NewOlt(parent *OltManeger, macAddr sources.HardwareAddr) *Olt {
	return &Olt{
		parent:    parent,
		Mac:       macAddr,
		ONUs:      make([]*ONU, 0),
		Log:       slog.New(parent.Log.Handler().WithAttrs([]slog.Attr{slog.String("olt", macAddr.String())})),
		onceStart: &sync.Once{},
	}
}

func (olt *Olt) SetAuthLoid(loid string) error {
	info, err := olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac,
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0d,
		Flag2:       0xff,
		Data:        []byte(loid),
	}, time.Second*2)
	if err == nil && info.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) SetAuthPass(pass string) error {
	info, err := olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac,
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0c,
		Flag2:       0xff,
		Data:        []byte(pass),
	}, time.Second*2)
	if err == nil && info.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) OltInfo() {
	for {
		pkt, err := olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get current temperature", "error", err)
			continue
		}
		olt.Temperature = (float64(binary.BigEndian.Uint16(pkt.Data)) / 100) * 2

		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag1: 0x04, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get max temperature", "error", err)
			continue
		}
		olt.MaxTemperature = (float64(binary.BigEndian.Uint16(pkt.Data)) / 100) * 2

		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get current ONUs online", "error", err)
			continue
		}
		olt.OnlineONU = pkt.Data[0]

		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x01, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get OLT uptime", "error", err)
			continue
		}
		olt.Uptime = max(0, ((time.Duration(binary.BigEndian.Uint64(pkt.Data)) * 16) - (time.Second * 2)).Round(time.Second))

		// Wait 5s to update info
		<-time.After(time.Second)
	}
}

func (olt *Olt) fetchONUInfo() {
	olt.ONUs = make([]*ONU, olt.MaxONU)
	for onuID := range olt.MaxONU {
		onu := new(ONU)
		onu.ID = onuID
		onu.Request = make(map[string]any)
		onu.Log = slog.New(olt.Log.Handler().WithAttrs([]slog.Attr{slog.Int("onu", int(onuID)+1)}))
		olt.ONUs[onuID] = onu
	}

	pkt := &sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01}
	for {
		for onuID := range olt.MaxONU {
			onu, pkt := olt.ONUs[onuID], pkt.Clone().SetFlag2(onuID)
			if res, err := olt.parent.pktSource.Send(pkt, time.Second); err == nil {
				onu.Status = ONUStatus(res.Data[0])
				if onu.Status == 0 {
					onu.SN = gponsn.Sn{}
					onu.Uptime = 0
					continue
				} else if err = onu.GetInfo(olt); err != nil {
					onu.Log.Log(olt.parent.context, slog.LevelError, "Error on process Info", "error", err)
				}
			}
		}
		<-time.After(time.Second)
	}
}

func (onu *ONU) GetInfo(olt *Olt) (err error) {
	pkt := &sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag2: onu.ID}
	res, err := olt.parent.pktSource.Send(pkt.SetFlag1(0x06), time.Second)
	if err != nil {
		return
	} else if onu.SN, err = gponsn.Parse(res.Data[:8]); err != nil {
		return
	}

	res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x10), time.Second)
	if err != nil {
		return
	}
	onu.Request["0x10"] = int8(res.Data[0])

	// ONU Connection time
	if res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x02), time.Second); err == nil {
		onu.Uptime = max(0, (olt.Uptime - time.Duration(binary.BigEndian.Uint64(res.Data)*16)).Round(time.Second))
	}

	if _, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x03)); err != nil {
		return
	}
	if _, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x0d), time.Second); err != nil {
		return
	}
	onu.Request["0x0d"] = hex.EncodeToString(res.Data)

	if _, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x07)); err != nil {
		return
	}
	onu.Request["0x07"] = hex.EncodeToString(res.Data[:4])

	if _, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x0f)); err != nil {
		return
	}
	onu.Request["0x0f"] = hex.EncodeToString(res.Data[:12])

	return
}

func (olt *Olt) startPkt(pkt *sources.Packet) {
	olt.onceStart.Do(func() {
		var err error
		olt.FirmwareVersion = strings.TrimFunc(string(pkt.Data), func(r rune) bool {
			return unicode.IsSpace(r) || r == 0x0
		})

		// Get max ONUs
		pkt, err = olt.parent.pktSource.Send(sources.New().SetMacAddr(pkt.Mac).SetRequestType(0xc).SetFlag0(0x1).SetFlag1(0x18).SetFlag2(0xff), time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get max ONUs", "error", err)
			return
		}
		olt.MaxONU = min(pkt.Data[0], MaxONU)
		olt.Log.Log(olt.parent.context, slog.LevelInfo, "Max ONUs", "max_onu", olt.MaxONU)

		// Get OLT DNA
		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x08, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get OLT DNA", "error", err)
			return
		}
		olt.DNA = hex.EncodeToString(pkt.Data[:bytes.IndexByte(pkt.Data, 0x0)])
		olt.Log.Log(olt.parent.context, slog.LevelInfo, "OLT DNA", "dna", olt.DNA)

		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x05, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x06, Flag2: 0xff})

		// Get current ONUs connected
		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get ONUs connected", "error", err)
			return
		}
		olt.OnlineONU = uint8(pkt.Data[0])
		olt.Log.Log(olt.parent.context, slog.LevelInfo, "Current ONU online", "online_onu", olt.OnlineONU)

		// Get current OMCI Mode
		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x19, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "error on get OMCI Mode", "error", err)
			return
		}
		olt.OMCIMode = int(byte(pkt.Data[0]))
		olt.Log.Log(olt.parent.context, slog.LevelInfo, "OMCI Mode", "mode", olt.OMCIMode)

		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x02, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x07, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0b, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0c, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0d, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x09, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x05, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x07, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0a, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x06, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x08, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x11, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x12, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x01, Flag1: 0x13, Flag2: 0xff})

		// ??
		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01, Flag2: 0x00}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "??", "error", err)
			return
		}

		// OLT Config
		pkt, err = olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000f, Flag1: 0x09, Flag2: 0xff}, time.Second)
		if err != nil {
			olt.Log.Log(olt.parent.context, slog.LevelError, "OLT Config error", "error", err)
			return
		}
		olt.Log.Log(olt.parent.context, slog.LevelInfo, "OLT Config", "config", hex.EncodeToString(pkt.Data))

		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x10, Flag2: 0xff})
		olt.parent.pktSource.Send(&sources.Packet{Mac: pkt.Mac, RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff})

		go olt.OltInfo()
		go olt.fetchONUInfo()
	})
}

func (olt *Olt) Packet(pkt *sources.Packet) {
	olt.Log.Log(olt.parent.context, slog.LevelInfo, "droped pkt", "requestID", pkt.RequestID, "data", pkt.Data)
}
