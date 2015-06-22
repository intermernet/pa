// pa - Port Authority - controls port assignments, ensures port uniqueness
// Copyright 2014 Mike Hughes intermernet AT gmail DOT com

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	limPort  = 65535 // TCP/IP Limit
	autoPort = 0
)

var (
	v *vendor

	minPort, maxPort int

	portFormat = "^/\\d{1,5}$"
	portRegExp = regexp.MustCompile(portFormat)

	listen = ":3000"

	config = "./pa.json"

	errInvalidRoute  = "Error: invalid route."
	errInvalidMethod = "Error: method not allowed."

	errMinOutOfRange = "Error: min out of range."
	errMaxOutOfRange = "Error: max out of range."
	errMinGTMax      = "Error: min cannot be greater than than max."

	errPortOutOfRange      = errors.New("Error: port out of range.")
	errAllPortsAssigned    = errors.New("Error: all Ports assigned.")
	errPortAlreadyAssigned = errors.New("Error: port is already assigned.")
)

func internalServerError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return
}

type vendor struct {
	Ports [limPort]uint32 `json:"ports"`
}

func (v *vendor) onIffOff(port int) bool {
	return atomic.CompareAndSwapUint32(&v.Ports[port-1], 0, 1)
}

func (v *vendor) offIffOn(port int) {
	atomic.CompareAndSwapUint32(&v.Ports[port-1], 1, 0)
	return
}

func (v *vendor) next() (int, error) {
	for n := range v.Ports[minPort:maxPort] {
		np := n + minPort
		if v.onIffOff(np) {
			return np, nil
		}
	}
	return 0, errAllPortsAssigned
}

func (v *vendor) assign(port int) (int, error) {
	if port == autoPort {
		return v.next()
	}
	if port < minPort || port > maxPort {
		return 0, errPortOutOfRange
	}
	if v.onIffOff(port) {
		return port, nil
	}
	return 0, errPortAlreadyAssigned
}

func (v *vendor) release(port int) (int, error) {
	if port < minPort || port > maxPort {
		return 0, errPortOutOfRange
	}
	v.offIffOn(port)
	return port, nil
}

func (v *vendor) get() (int, error) {
	port, err := v.assign(autoPort)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func (v *vendor) post(port int) (int, error) {
	port, err := v.assign(port)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func (v *vendor) del(port int) (int, error) {
	return v.release(port)
}

func (v *vendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		if r.Method == "GET" {
			port, err := v.get()
			if err != nil {
				internalServerError(w, err)
				return
			}
			w.Write([]byte(fmt.Sprintf("%d", port)))
			return
		}
		http.Error(w, errInvalidMethod, http.StatusMethodNotAllowed)
		return
	case portRegExp.Match([]byte(r.URL.Path)):
		switch r.Method {
		case "POST":
			port, err := strconv.Atoi(r.URL.Path[1:])
			if err != nil {
				internalServerError(w, err)
				return
			}
			port, err = v.post(port)
			if err != nil {
				internalServerError(w, err)
				return
			}
			w.Write([]byte(fmt.Sprintf("%d", port)))
			return
		case "DELETE":
			port, err := strconv.Atoi(r.URL.Path[1:])
			if err != nil {
				internalServerError(w, err)
				return
			}
			port, err = v.del(port)
			if err != nil {
				internalServerError(w, err)
				return
			}
			w.Write([]byte(fmt.Sprintf("%d", port)))
			return
		default:
			http.Error(w, errInvalidMethod, http.StatusMethodNotAllowed)
			return
		}
	default:
		http.Error(w, errInvalidRoute, http.StatusNotFound)
		return
	}
}

func init() {
	flag.IntVar(&minPort, "min", 9000, "lowest TCP/IP Port to distribute (default=9000)")
	flag.IntVar(&maxPort, "max", limPort, fmt.Sprintf("highest TCP/IP Port to distribute (default=%d)", limPort))
	flag.Parse()
	if minPort < 1 {
		log.Fatalf("%s\n", errMinOutOfRange)
	}
	if maxPort > limPort {
		log.Fatalf("%s\n", errMaxOutOfRange)
	}
	if minPort > maxPort {
		log.Fatalf("%s\n", errMinGTMax)
	}
	var ports [limPort]uint32
	v = &vendor{Ports: ports}
	f, err := os.Open(config)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config file not found. Creating new config %s\n", config)
			f, err = os.Create(config)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal(err)
		}
	} else {
		fStat, err := f.Stat()
		if err != nil {
			log.Fatal(err)
		}
		confJSON := make([]byte, fStat.Size())
		if _, err = f.Read(confJSON); err != nil {
			log.Fatal(err)
		}
		log.Printf("Loading config from %s\n", config)
		if err = json.Unmarshal(confJSON, &v); err != nil {
			log.Printf("Error reading config file: %s. Creating new config %s\n", err.Error(), config)
			f, err = os.Create(config)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	server := &http.Server{
		Addr:         listen,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	signalChan := make(chan os.Signal, 1)
	quit := make(chan struct{})
	defer close(signalChan)
	defer close(quit)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	go func() {
		for _ = range signalChan {
			log.Println("Saving data...")
			data, err := json.Marshal(v)
			if err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(config)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			if _, err = f.Write(data); err != nil {
				log.Fatal(err)
			}
			log.Println("Data saved to", config)
			signal.Stop(signalChan)
			server.SetKeepAlivesEnabled(false)
			quit <- struct{}{}
		}
	}()
	http.Handle("/", v)
	log.Printf("Listening on %s\n", listen)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("ListenAndServe: %s\n", err)
		}
	}()
	log.Println("Press Ctrl-C to quit")
	<-quit
	log.Println("Exiting")
	os.Exit(0)
}
