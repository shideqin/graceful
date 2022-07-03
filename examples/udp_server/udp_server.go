package main

import (
	"context"
	"fmt"
	"log"
	"net"

	mdns "github.com/miekg/dns"
	"github.com/shideqin/graceful"
)

func main() {
	// start udp server
	ln, err := graceful.ListenUDP("udp", ":53")
	if err != nil {
		log.Fatalf("graceful.ListenUDP error: %v", err)
	}
	mdns.HandleFunc(".", handle)
	server := &mdns.Server{PacketConn: ln}
	go func() {
		err = server.ActivateAndServe()
		if err != nil && err != net.ErrClosed {
			log.Printf("server.ActivateAndServe error: %v\n", err)
		}
	}()

	// graceful
	graceful.HandleSignal(func(ctx context.Context) {
		err = server.ShutdownContext(ctx)
		if err != nil {
			log.Printf("dnsServe.ShutdownContext error: %v\n", err)
		}
	})
}

func handle(w mdns.ResponseWriter, r *mdns.Msg) {
	go func() {
		remote := w.RemoteAddr().String()
		m := new(mdns.Msg)
		m.SetReply(r)
		for _, q := range m.Question {
			fmt.Println(q.Name, remote)
		}
		_ = w.WriteMsg(m)
	}()
}
