package packet

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"encoding/hex"
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
	*hd = make(HardwareAddr, len(mac))
	copy((*hd)[:], mac)
	return nil
}

type Packet struct {
	RequestID uint16 `json:"request"`
	Type      uint16 `json:"id"`
	Flag0     uint8  `json:"reserv"`
	Flag1     uint8  `json:"check0"`
	Flag2     uint8  `json:"check1"`
	Status    uint8  `json:"status"`

	Mac    HardwareAddr `json:"mac_addr"`
	Header []byte       `json:"-"`
	Data   []byte       `json:"data"`
}

func IsOltPacket(raw []byte) bool {
	if len(raw) < 50 {
		return false
	}
	return bytes.HasPrefix(raw, OltMagic)
}

func (pkt *Packet) Unmarshal(mac HardwareAddr, raw []byte) error {
	if err := pkt.UnmarshalBinary(raw); err != nil {
		return err
	}
	pkt.Mac = mac
	return nil
}

func (pkt *Packet) UnmarshalBinary(raw []byte) error {
	if len(raw) < 50 {
		return ErrNotValid
	}

	if raw, ok := bytes.CutPrefix(raw, OltMagic); ok {
		header, raw := raw[:8], raw[8:]
		pkt.Header = header
		pkt.Data = raw

		pkt.RequestID = binary.BigEndian.Uint16(header[:2])
		pkt.Type = binary.BigEndian.Uint16(header[2:4])
		pkt.Status = uint8(header[4])
		pkt.Flag0 = uint8(header[5])
		pkt.Flag1 = uint8(header[6])
		pkt.Flag2 = uint8(header[7])
		return nil
	}

	return ErrNoMagic
}

func (pkt Packet) MarshalBinary() ([]byte, error) {
	header := make([]byte, len(OltMagic), 50)
	copy(header, OltMagic)
	header = binary.BigEndian.AppendUint16(header, pkt.RequestID)
	header = binary.BigEndian.AppendUint16(header, pkt.Type)
	header = append(header, byte(pkt.Status), byte(pkt.Flag0), byte(pkt.Flag1), byte(pkt.Flag2))

	if pkt.Data != nil {
		copy(header[len(header):cap(header)], pkt.Data)
	}

	return header[:cap(header)], nil
}

func (pkt Packet) String() string {
	return fmt.Sprintf("Request %d - Type 0x%x (0x%x), Status 0x%x (%s)", pkt.RequestID, pkt.Type, pkt.Id(), pkt.Status, hex.EncodeToString(pkt.Header))
}

func (pkt Packet) Id() int {
	return int(pkt.Type)>>(int(pkt.Flag0)>>8) + (int(pkt.Flag1) | int(pkt.Flag2))
}
