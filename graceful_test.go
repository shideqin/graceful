package graceful

import (
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/miekg/dns"
	mdns "github.com/miekg/dns"
)

func handleError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal("failed", err)
	}
}

func TestListenTCP(t *testing.T) {
	ln, err := ListenTCP("tcp", ":8080")
	handleError(t, err)
	defer ln.Close()

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	})
	go func() {
		_ = http.Serve(ln, nil)
	}()

	resp, err := http.Get("http://" + ln.Addr().String() + "/hello")
	handleError(t, err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	handleError(t, err)

	if string(body) != "hello world" {
		t.Fatal("expected hello world, but got", string(body))
	}
}

func TestListenUDP(t *testing.T) {
	//server
	ln, err := ListenUDP("udp", ":53")
	handleError(t, err)

	mdns.HandleFunc(".", func(w mdns.ResponseWriter, r *mdns.Msg) {
		m := new(mdns.Msg)
		m.SetReply(r)
		for _, q := range m.Question {
			qName := strings.ToLower(q.Name)
			rrHeader := dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			}
			a := &dns.A{Hdr: rrHeader, A: net.ParseIP("127.0.0.1")}
			m.Answer = append(m.Answer, a)
		}
		_ = w.WriteMsg(m)
	})
	server := &mdns.Server{PacketConn: ln}

	go func() {
		err = server.ActivateAndServe()
		handleError(t, err)
	}()

	//client
	c := new(mdns.Client)
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn("localhost"), mdns.TypeA)
	_, _, err = c.Exchange(m, "localhost:53")
	handleError(t, err)
}
