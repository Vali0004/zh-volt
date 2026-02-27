package sources

import (
	"iter"
	"net"
)

type PacketRaw struct {
	Data   []byte
	MacSrc net.HardwareAddr
}

type Sources interface {
	Close() error                                           // Stop process packets
	MacAddr() net.HardwareAddr                              // Hardware mac address if exist
	GetPacketData() iter.Seq2[*PacketRaw, error]            // Get data
	SendPacketData(dst net.HardwareAddr, data []byte) error // Send data packet
}
