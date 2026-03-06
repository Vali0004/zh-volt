package sources

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

var (
	_ encoding.BinaryMarshaler   = &Packet{}
	_ encoding.BinaryUnmarshaler = &Packet{}

	_ encoding.TextMarshaler   = HardwareAddr{}
	_ encoding.TextUnmarshaler = &HardwareAddr{}

	ErrNotValid = errors.New("packet not valid")
	ErrNoMagic  = errors.New("packet have magic in star section")
	OltMagic    = []byte{0xb9, 0x58, 0xd6, 0x3a}
)

type HardwareAddr net.HardwareAddr

func (hd HardwareAddr) Net() net.HardwareAddr { return net.HardwareAddr(hd) }

func (hd HardwareAddr) String() string { return net.HardwareAddr(hd).String() }

func (hd HardwareAddr) MarshalText() ([]byte, error) {
	return []byte(hd.String()), nil
}

func (hd *HardwareAddr) UnmarshalText(text []byte) error {
	mac, err := net.ParseMAC(string(text))
	if err != nil {
		return err
	}
	*hd = HardwareAddr(mac)
	return nil
}

type Packet struct {
	RequestID   uint16 `json:"request_id"`
	RequestType uint16 `json:"request_type"`
	Flag0       uint8  `json:"flag0"`
	Flag1       uint8  `json:"flag1"`
	Flag2       uint8  `json:"flag2"`
	Flag3       uint8  `json:"flag3"` // Old `Status` flag

	Error  error        `json:"error"`
	Mac    HardwareAddr `json:"mac"`
	Header []byte       `json:"-"`
	Data   []byte       `json:"data"`
}

func IsOltPacket(raw []byte) bool {
	return bytes.HasPrefix(raw, OltMagic)
}

func Parse[K HardwareAddr | net.HardwareAddr](mac K, data []byte) (*Packet, error) {
	pkt := &Packet{Mac: HardwareAddr(mac)}
	return pkt, pkt.UnmarshalBinary(data)
}

func (pkt *Packet) UnmarshalBinary(raw []byte) error {
	if !IsOltPacket(raw) {
		return ErrNotValid
	}

	if raw, ok := bytes.CutPrefix(raw, OltMagic); ok {
		header, raw := raw[:8], raw[8:]
		pkt.Header = header
		pkt.Data = raw

		pkt.RequestID = binary.BigEndian.Uint16(header[:2])
		pkt.RequestType = binary.BigEndian.Uint16(header[2:4])
		pkt.Flag3 = uint8(header[4])
		pkt.Flag0 = uint8(header[5])
		pkt.Flag1 = uint8(header[6])
		pkt.Flag2 = uint8(header[7])
		return nil
	}

	return ErrNoMagic
}

func (pkt Packet) Encode() []byte {
	header := make([]byte, len(OltMagic), 50)
	copy(header, OltMagic)
	header = binary.BigEndian.AppendUint16(header, pkt.RequestID)
	header = binary.BigEndian.AppendUint16(header, pkt.RequestType)
	header = append(header, byte(pkt.Flag3), byte(pkt.Flag0), byte(pkt.Flag1), byte(pkt.Flag2))

	if pkt.Data != nil {
		copy(header[len(header):cap(header)], pkt.Data)
	}

	return header[:cap(header)]
}

func (pkt Packet) MarshalBinary() ([]byte, error) { return pkt.Encode(), nil }
func (pkt Packet) String() string {
	return fmt.Sprintf("%02x-%02x-%02x-%02x", pkt.Flag0, pkt.Flag1, pkt.Flag2, pkt.Flag3)
}

// Return new Packet with Broadcast MAC Address
func New() *Packet {
	return &Packet{
		Mac: bytes.Clone(BroadcastMAC),
	}
}

func (pkt Packet) Clone() *Packet {
	return &Packet{
		RequestID:   pkt.RequestID,
		RequestType: pkt.RequestType,
		Flag0:       pkt.Flag0,
		Flag1:       pkt.Flag1,
		Flag2:       pkt.Flag2,
		Flag3:       pkt.Flag3,

		Error: pkt.Error,
		Mac:   bytes.Clone(pkt.Mac),
		Data:  bytes.Clone(pkt.Data),
	}
}

func (pkt *Packet) SetMacAddr(mac HardwareAddr) *Packet {
	pkt.Mac = bytes.Clone(mac)
	return pkt
}

func (pkt *Packet) SetNetMacAddr(mac net.HardwareAddr) *Packet {
	pkt.Mac = HardwareAddr(mac)
	return pkt
}

func (pkt *Packet) SetRequestID(data uint16) *Packet {
	pkt.RequestID = data
	return pkt
}

func (pkt *Packet) SetRequestType(data uint16) *Packet {
	pkt.RequestType = data
	return pkt
}

func (pkt *Packet) SetFlag0(data uint8) *Packet {
	pkt.Flag0 = data
	return pkt
}

func (pkt *Packet) SetFlag1(data uint8) *Packet {
	pkt.Flag1 = data
	return pkt
}

func (pkt *Packet) SetFlag2(data uint8) *Packet {
	pkt.Flag2 = data
	return pkt
}

func (pkt *Packet) SetFlag3(data uint8) *Packet {
	pkt.Flag3 = data
	return pkt
}
