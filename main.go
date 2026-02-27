package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/sources/pcap"
)

func main() {
	defer fmt.Fprint(os.Stderr, "\nexiting process!\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	ifaceName := "eth0" // needs replace
	pcapSource, err := pcap.New(ifaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open pcap for %s: %s", ifaceName, err)
		os.Exit(1)
	}
	defer pcapSource.Close()

	olts, err := zhvolt.NewOltProcess(pcapSource, os.Stdout, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create olt process: %s", err)
		os.Exit(1)
	}
	defer olts.Close()

	go http.ListenAndServe(":8081", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := olts.GetOlts()
		w.Header().Set("Content-Type", "application/json; utf-8")
		js := json.NewEncoder(w)
		js.SetIndent("", "  ")
		if err = js.Encode(data); err != nil {
			olts.Log.Printf("error on encode olt: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on encode olt data: %s\n\n", err)
		}
	}))

	<-ctx.Done()
}
