package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	fwd := &forwarder{"words", 8080}
	http.Handle("/words/", http.StripPrefix("/words", fwd))
	http.Handle("/", http.FileServer(http.Dir("static")))

	fmt.Println("Listening on port 80")
	err := http.ListenAndServe(":80", nil)
	if err != nil {
		fmt.Printf("Error while starting server: %v", err)
	}
}

type forwarder struct {
	host string
	port int
}

func (f *forwarder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	addrs, err := net.LookupHost(f.host)
	if err != nil {
		log.Println("Error", err)
		http.Error(w, err.Error(), 500)
		return
	}

	log.Printf("%s %d available ips: %v", r.URL.Path, len(addrs), addrs)
	ip := addrs[rand.Intn(len(addrs))]
	log.Printf("%s I choose %s", r.URL.Path, ip)

	url := fmt.Sprintf("http://%s:%d%s", ip, f.port, r.URL.Path)
	log.Printf("%s Calling %s", r.URL.Path, url)

	if err = copy(url, ip, w); err != nil {
		log.Println("Error", err)
		http.Error(w, err.Error(), 500)
		return
	}
}

func copy(url, ip string, w http.ResponseWriter) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	w.Header().Set("source", ip)

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	_, err = w.Write(buf)
	return err
}
