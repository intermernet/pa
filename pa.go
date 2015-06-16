// pa - Port Authority - controls port assignments, ensures port uniqueness
// Copyright 2014 Mike Hughes intermernet AT gmail DOT com

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync"
)

const (
	minPort  = 9000
	maxPort  = 65535 // TCP/IP Limit
	autoPort = maxPort + 1
)

var (
	v          *vendor
	portFormat = "^/\\d{1,5}$"
	portRegExp = regexp.MustCompile(portFormat)

	listen = ":3000"

	config = "./pa.json"

	errInvalidRoute  = "Error: invalid route."
	errInvalidMethod = "Error: method not allowed."

	errPortOutOfRange      = errors.New("Error: port out of range.")
	errAllPortsAssigned    = errors.New("Error: all Ports assigned.")
	errPortAlreadyAssigned = errors.New("Error: port is already assigned.")
)

type vendor struct {
	Ports []bool `json:"ports"`
	sync.Mutex
}

func (v *vendor) scan() (int, error) {
	for n, assigned := range v.Ports {
		if !assigned {
			return n, nil
		}
	}
	return 0, errAllPortsAssigned
}

func (v *vendor) assign(port int) (int, error) {
	if port <= maxPort && v.Ports[port-minPort] {
		return 0, errPortAlreadyAssigned
	}
	if port == autoPort {
		p, err := v.scan()
		if err != nil {
			return 0, err
		}
		port = p + minPort
	}
	v.Ports[port-minPort] = true
	return port, nil
}

func (v *vendor) get() (int, error) {
	v.Lock()
	defer v.Unlock()
	port, err := v.assign(autoPort)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func (v *vendor) post(port int) (int, error) {
	v.Lock()
	defer v.Unlock()
	if port < minPort || port > maxPort {
		return 0, errPortOutOfRange
	}
	port, err := v.assign(port)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func (v *vendor) del(port int) (int, error) {
	v.Lock()
	defer v.Unlock()
	if port < minPort || port > maxPort {
		return 0, errPortOutOfRange
	}
	v.Ports[port-minPort] = false
	return port, nil
}

func (v *vendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		if r.Method == "GET" {
			port, err := v.get()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			port, err = v.post(port)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Write([]byte(fmt.Sprintf("%d", port)))
			return
		case "DELETE":
			port, err := strconv.Atoi(r.URL.Path[1:])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			port, err = v.del(port)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
	ports := make([]bool, maxPort-minPort+1, maxPort-minPort+1)
	v = &vendor{Ports: ports}
	f, err := os.Open(config)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config file not found. Creating %s\n", config)
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
		_, err = f.Read(confJSON)
		if err != nil {
			log.Fatal(err)
		}
		err = json.Unmarshal(confJSON, &v)
		if err != nil {
			log.Printf("Config file corrupt (JSON Error: %s). Creating %s\n", err.Error(), config)
			f, err = os.Create(config)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	err = f.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
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
			_, err = f.Write(data)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Data saved to", config)
			quit <- struct{}{}
		}
	}()
	http.Handle("/", v)
	log.Printf("Listening on %s\n", listen)
	go func() {
		if err := http.ListenAndServe(listen, nil); err != nil {
			log.Fatalf("ListenAndServe: %s\n", err)
		}
	}()
	log.Println("Press Ctrl-C to quit")
	<-quit
	log.Println("Exiting")
	os.Exit(0)
}
