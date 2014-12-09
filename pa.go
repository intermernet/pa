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
	"strconv"
	"sync"

	"github.com/gorilla/pat"
)

const (
	minPort  = 9000
	maxPort  = 65535 // TCP/IP Limit
	autoPort = maxPort + 1
)

var (
	V *vendor

	portFormat = "/{port:[0-9]+}"

	listen = ":3000"

	config = "./config.json"

	ErrPortOutOfRange      = errors.New("Error: port out of range.")
	ErrAllPortsAssigned    = errors.New("Error: all Ports assigned.")
	ErrPortAlreadyAssigned = errors.New("Error: port is already assigned.")
)

type vendor struct {
	Ports []bool
	sync.Mutex
}

func rangeCheck(port int) error {
	if port < minPort || port > maxPort {
		return ErrPortOutOfRange
	}
	return nil
}

func (v *vendor) scan() (int, error) {
	for n, assigned := range v.Ports {
		if !assigned {
			return n, nil
		}
	}
	return 0, ErrAllPortsAssigned
}

func (v *vendor) assign(port int) (int, error) {
	if port <= maxPort && v.Ports[port-minPort] {
		return 0, ErrPortAlreadyAssigned
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
	if err := rangeCheck(port); err != nil {
		return 0, err
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
	if err := rangeCheck(port); err != nil {
		return 0, err
	}
	v.Ports[port-minPort] = false
	return port, nil
}

func (v *vendor) getHandler(w http.ResponseWriter, r *http.Request) {
	port, err := v.get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(fmt.Sprintf("%d", port)))
}

func (v *vendor) postHandler(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(r.URL.Query().Get(":port"))
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
}

func (v *vendor) delHandler(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(r.URL.Query().Get(":port"))
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
}

func init() {
	Ports := make([]bool, maxPort-minPort+1, maxPort-minPort+1)
	V = &vendor{Ports: Ports}
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
		fi, err := f.Stat()
		if err != nil {
			log.Fatal(err)
		}
		confJSON := make([]byte, fi.Size())
		_, err = f.Read(confJSON)
		if err != nil {
			log.Fatal(err)
		}
		err = json.Unmarshal(confJSON, &V)
		if err != nil {
			log.Fatal(err)
		}
	}
	err = f.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	c := make(chan os.Signal, 1)
	defer close(c)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		for _ = range c {
			log.Println("Saving data...")
			data, err := json.Marshal(V)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
			f, err := os.Create(config)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
			defer f.Close()
			_, err = f.Write(data)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
			log.Println("Exiting.")
			os.Exit(0)
		}
	}()

	r := pat.New()
	r.Get("/", http.HandlerFunc(V.getHandler))
	r.Post(portFormat, http.HandlerFunc(V.postHandler))
	r.Delete(portFormat, http.HandlerFunc(V.delHandler))
	http.Handle("/", r)
	log.Printf("Listening on %s\n", listen)
	if err := http.ListenAndServe(listen, nil); err != nil {
		log.Fatalf("ListenAndServe: %s\n", err)
	}
}
