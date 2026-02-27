package zhvolt

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/gponsn"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/packet"
)

type ONUStatus uint8

type ONU struct {
	ID         uint8          `json:"id"`
	Status     ONUStatus      `json:"status"`
	LastUpdate time.Time      `json:"last_update"`
	SN         gponsn.GPON_SN `json:"gpon_sn"`
	TxPower    float64        `json:"tx_power"`
	RxPower    float64        `json:"rx_power"`
	Voltage    uint64         `json:"voltage"`
	Current    uint64         `json:"current"`
	Temp       uint64         `json:"temperature"`
}

type Olt struct {
	Up              time.Time           `json:"up"`
	MacAddr         packet.HardwareAddr `json:"mac_addr"`
	FirmwareVersion string              `json:"fw_ver"`
	DNA             string              `json:"dna"`
	Temp            float64             `json:"temperature"`
	MaxTemp         float64             `json:"max_temperature"`

	OMCIMode  int   `json:"omci_mode"`
	OMCIErr   int   `json:"omci_err"`
	OnlineONU uint8 `json:"online_onu"`
	MaxONU    uint8 `json:"max_onu"`

	AuthLoid string `json:"auth_loid"`
	AuthPass string `json:"auth_passsword"`

	RogueONU uint `json:"rogue_onu"`

	ONUs map[uint8]*ONU `json:"onu"`

	Log    *log.Logger `json:"-"`
	parent *Maneger    `json:"-"`
}

func (olt *Olt) SendPkt(pkt packet.Packet) error {
	pkt.Mac = olt.MacAddr
	return olt.parent.SendRequestPacket(pkt)
}

func (olt *Olt) SendPktFn(pkt packet.Packet, fn RequestCallback) error {
	pkt.Mac = olt.MacAddr
	return olt.parent.SendRequestPacketCall(pkt, false, fn)
}

func (olt *Olt) Onu() {
	olt.SendPkt(packet.Packet{Type: 0x000c, Flag1: 0x10, Flag2: 0xff})
	olt.SendPktFn(packet.Packet{Type: 0x000c, Flag1: 0x03, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {})

	olt.parent.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x18, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
		olt.MaxONU = uint8(pkt.Data[0])
		for onuID := range olt.MaxONU {
			onu := &ONU{}
			// onu.SN = new(gponsn.GPON_SN)
			onu.ID = onuID + 0x1
			olt.ONUs[onu.ID] = onu

			var process RequestCallback
			process = func(pkt packet.Packet, olt *Olt) {
				switch err := onu.SN.UnmarshalText(pkt.Data); err {
				case nil:
					if onu.Status == 0 {
						olt.Log.Printf("ONU Online SN: %s", onu.SN)
					}
					onu.Status = 1
				case gponsn.ErrVendorName:
					olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x00, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}, process)
					return
				case gponsn.ErrNullSn:
					olt.Log.Printf("onu %d: %s", onu.ID, hex.EncodeToString(pkt.Data))
					if onu.Status >= 1 {
						olt.Log.Printf("ONU %d update to offline", onu.ID)
					}
					onu.Status = 0
					olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x00, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}, process)
					return
				default:
					olt.Log.Printf("ONU Error %d: %s", onu.ID, err)
					olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x00, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}, process)
					return
				}

				olt.SendPktFn(packet.Packet{Type: 0x000c, Flag0: 0x02, Flag1: 0x02, Flag2: onu.ID}, func(pkt packet.Packet, olt *Olt) {
					// Ignore

					olt.SendPktFn(packet.Packet{Type: 0x000c, Flag0: 0x02, Flag1: 0x10, Flag2: uint8(onu.ID)}, func(pkt packet.Packet, olt *Olt) {
						lastUpdate := binary.BigEndian.Uint32(pkt.Data[2:6])
						onu.LastUpdate = olt.Up.Add(time.Duration(lastUpdate))

						olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x0, Flag0: 0x2, Flag1: 0x03, Flag2: onu.ID}, func(pkt packet.Packet, olt *Olt) {
							olt.Log.Printf("ONU %d: %s", onu.SN, hex.EncodeToString(pkt.Data))
						})
						olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x0, Flag0: 0x2, Flag1: 0x0d, Flag2: onu.ID}, func(pkt packet.Packet, olt *Olt) {})

						// <-time.After(time.Microsecond * 500)
						// Send again
						olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x00, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}, process)
					})
				})
			}
			olt.SendPktFn(packet.Packet{Type: 0x000c, Status: 0x00, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}, process)
		}
	})

	for olt.parent.run.Load() {
		olt.parent.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag1: 0x03, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
			olt.Temp = float64(binary.BigEndian.Uint16(pkt.Data))
			olt.Temp /= 100
			olt.Temp *= 2
		})
		olt.parent.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag1: 0x04, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
			olt.MaxTemp = float64(binary.BigEndian.Uint16(pkt.Data))
			olt.MaxTemp /= 100
			olt.MaxTemp *= 2
		})
		olt.parent.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x02, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
			olt.OnlineONU = uint8(pkt.Data[0])
			olt.parent.SendPktBroadcastFn(packet.Packet{Type: 0x000c, Flag0: 0x01, Flag1: 0x19, Flag2: 0xff}, func(pkt packet.Packet, olt *Olt) {
				olt.OMCIMode = int(byte(pkt.Data[0]))
			})
		})

		<-time.After(time.Millisecond * 300)
	}
}

func (olt *Olt) ProcessPkt(pkt packet.Packet) {
	if data, err := json.MarshalIndent(pkt, "", "  "); err == nil {
		olt.Log.Printf("No processed pkt: %s", string(data))
	}
}
