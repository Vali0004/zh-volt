package olt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"sync"
	"time"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

const (
	defaultLogFlag = log.Ldate | log.Ltime | log.LUTC | log.Lmsgprefix
	u16Limit       = ^uint16(0)

	MaxONU uint8 = 128
)

var (
	BroadcastMAC = sources.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	ErrCallbackTimeout = errors.New("timeout on callback call")
)

type oltManegerCallback func(pkt *sources.PacketRaw, olt *Olt, removeReqID func())

type OltManeger struct {
	CorrentRequest uint16
	Log            *log.Logger
	Verbose        uint8

	context   context.Context
	pktSource sources.Sources
	newDev    chan struct{}

	reqLock     *sync.Mutex
	oltCallback map[uint16]oltManegerCallback
	olt         *util.SyncMap[string, *Olt]
	onceStart   *sync.Once
}

func NewOltProcess(ctx context.Context, source sources.Sources, logWrite io.Writer) (*OltManeger, error) {
	oltManeger := &OltManeger{
		context:        ctx,
		pktSource:      source,
		Log:            log.New(logWrite, fmt.Sprintf("OLT Maneger %s: ", source.MacAddr()), defaultLogFlag),
		Verbose:        1,
		newDev:         make(chan struct{}, 1),
		CorrentRequest: 0,
		olt:            util.NewSyncMap[string, *Olt](),
		onceStart:      &sync.Once{},
		oltCallback:    map[uint16]oltManegerCallback{},
		reqLock:        &sync.Mutex{},
	}

	return oltManeger, nil
}

func (man OltManeger) Olts() map[string]*Olt {
	return man.olt.Clone()
}

func (maneger *OltManeger) Close() (err error) {
	if maneger.pktSource != nil {
		if err = maneger.pktSource.Close(); err != nil {
			return
		}
	}
	if maneger.newDev != nil {
		select {
		case <-maneger.newDev:
		default:
			close(maneger.newDev)
			maneger.newDev = nil
		}
	}
	return
}

// Wait for fist device locate
func (man *OltManeger) Wait(timeout time.Duration) bool {
	if man.newDev == nil {
		return true
	}

	select {
	case <-man.newDev:
		return true
	case <-time.After(timeout):
		man.Close()
		return false
	}
}

func (man *OltManeger) assignerRequestID(raw *sources.PacketRaw) (overflow bool) {
	man.reqLock.Lock()
	defer man.reqLock.Unlock()

	if u16Limit == man.CorrentRequest {
		overflow = true
	}
	man.CorrentRequest++
	raw.Pkt.RequestID = man.CorrentRequest
	return
}

func (man *OltManeger) sendPacketCallback(raw *sources.PacketRaw, call oltManegerCallback) error {
	if man.assignerRequestID(raw) {
		for id := range man.oltCallback {
			delete(man.oltCallback, id)
		}
	}
	man.oltCallback[raw.Pkt.RequestID] = call
	if man.Verbose > 3 {
		man.Log.Printf("Seding to %s, reqID %d", raw.Mac, raw.Pkt.RequestID)
	}
	return man.pktSource.SendPkt(raw)
}

func (man *OltManeger) processPacket(workerID int) {
	defer man.Close()
	log := log.New(man.Log.Writer(), fmt.Sprintf("worker %d, %s", workerID+1, man.Log.Prefix()), man.Log.Flags())
	for pkt := range man.pktSource.GetPkts() {
		if man.Verbose > 3 {
			if pkt.Error != nil {
				man.Log.Printf("Worker %d: Pkt %s, Error %s", workerID, pkt.Mac, pkt.Error)
			} else {
				man.Log.Printf("Worker %d: Pkt %s, requestID %d", workerID, pkt.Mac, pkt.Pkt.RequestID)
			}
		}

		if pkt.Pkt.RequestID == u16Limit && len(man.oltCallback) > 0 {
			man.Log.Printf("Cleaning olt callbacks (%d)", len(man.oltCallback))
			for id := range man.oltCallback {
				delete(man.oltCallback, id)
			}
		}

		if pkt.Error != nil {
			log.Printf("Error on get packet from %s: %s", pkt.Mac, pkt.Error)
			continue
		}

		olt, exist := man.olt.Get(pkt.Mac.String())
		if !exist {
			log.Printf("New olt, Mac address %s", pkt.Mac)
			olt = NewOlt(man, pkt.Mac)
			man.olt.Set(pkt.Mac.String(), olt)
			if man.newDev != nil {
				man.newDev <- struct{}{}
				close(man.newDev)
				man.newDev = nil
			}
		}

		if fn, ok := man.oltCallback[pkt.Pkt.RequestID]; ok {
			fn(pkt, olt, func() {
				delete(man.oltCallback, pkt.Pkt.RequestID)
			})
			continue
		}

		olt.Packet(pkt)
	}
}

// Process packets in background
func (man *OltManeger) Start() {
	man.onceStart.Do(func() {
		cores := runtime.NumCPU() * 2
		man.Log.Printf("Starting process packet with %d workers", cores)
		for i := range cores {
			go man.processPacket(i)
		}

		man.sendPacketCallback(&sources.PacketRaw{
			Mac: BroadcastMAC,
			Pkt: &sources.Packet{
				RequestType: 0x000c,
				Flag2:       0xff,
			},
		}, func(pkt *sources.PacketRaw, olt *Olt, _ func()) {
			olt.startPkt(pkt)
		})
	})
}
