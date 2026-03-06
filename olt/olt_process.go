package olt

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

const MaxONU uint8 = 128

var (
	BroadcastMAC       = sources.BroadcastMAC
	ErrCallbackTimeout = errors.New("timeout on callback call")
)

type OltManeger struct {
	Log     *slog.Logger
	Verbose uint8

	context   context.Context
	pktSource sources.Sources
	newDev    chan struct{}

	olt       *util.SyncMap[string, *Olt]
	onceStart *sync.Once
}

func NewOltProcess(ctx context.Context, source sources.Sources, slogLevel slog.Level, logWrite io.Writer) (*OltManeger, error) {
	oltManeger := &OltManeger{
		context:   ctx,
		pktSource: source,
		Log:       slog.New(slog.NewJSONHandler(logWrite, &slog.HandlerOptions{Level: slogLevel})),
		Verbose:   1,
		newDev:    make(chan struct{}, 1),
		olt:       util.NewSyncMap[string, *Olt](),
		onceStart: &sync.Once{},
	}
	source.Slog(oltManeger.Log)
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

func (man *OltManeger) processPacket(workerID int) {
	defer man.Close()
	log := slog.New(man.Log.Handler().WithAttrs([]slog.Attr{slog.Int("id", workerID+1)}))
	log.Log(man.context, slog.LevelInfo, "Starting process packet")
	for pkt := range man.pktSource.GetPkts() {
		if pkt.Error != nil {
			log.Log(man.context, slog.LevelError, "Error on process packet", "error", pkt.Error)
			continue
		} else {
			log.Log(man.context, slog.LevelDebug, "Pkt received", "data", pkt.Data)
		}

		olt, exist := man.olt.Get(pkt.Mac.String())
		if !exist {
			log.Log(man.context, slog.LevelInfo, "New olt", "Mac address", pkt.Mac)
			olt = NewOlt(man, pkt.Mac)
			man.olt.Set(pkt.Mac.String(), olt)
			if man.newDev != nil {
				man.newDev <- struct{}{}
				close(man.newDev)
				man.newDev = nil
			}
		}

		olt.Packet(pkt)
	}
}

// Process packets in background
func (man *OltManeger) Start() {
	man.onceStart.Do(func() {
		cores := runtime.NumCPU() * 2
		for i := range cores {
			go man.processPacket(i)
		}

		man.pktSource.AsyncSend(sources.New().SetRequestType(0xc).SetFlag2(0xff), func(pkt *sources.Packet) bool {
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
			olt.startPkt(pkt)

			return false
		})
	})
}
