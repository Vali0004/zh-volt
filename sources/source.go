package sources

import (
	"log/slog"
	"net"
	"time"
)

const LimitForU16 = ^uint16(0)

var (
	BroadcastMAC = HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	ErrTimeout net.Error = &errTimeout{}
)

type errTimeout struct{}

func (errTimeout) Error() string   { return "i/o timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }

type ASyncFn func(pkt *Packet) (removeCallback bool)

// Generic interface to Send and Recive OLT Packets
type Sources interface {
	Close() error            // Stop process packets
	MacAddr() HardwareAddr   // Hardware mac address if exist
	GetPkts() <-chan *Packet // Get data
	Slog(log *slog.Logger)   // Add slog to Source

	// Send Packet, if timeout set Wait for packet response
	Send(pkt *Packet, timeout ...time.Duration) (*Packet, error)

	// Send packet and process in background
	AsyncSend(pkt *Packet, fn ASyncFn)
}
