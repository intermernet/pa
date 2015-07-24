// pa - Port Authority - controls port assignments, ensures port uniqueness
// Copyright 2014 Mike Hughes intermernet AT gmail DOT com
package main // import "intermer.net/go/pa"

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
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	limPort  = 65535 // TCP/IP Limit
	autoPort = 0     // Magic number for auto-assignment
)

var (
	v *vendor

	minPort, maxPort int

	portFormat = "^/\\d{1,5}$"
	portRegExp = regexp.MustCompile(portFormat)

	listen = ":3000"

	config = "./pa.json"

	errInvalidRoute  = "error: invalid route"
	errInvalidMethod = "error: method not allowed"

	errMinOutOfRange = "error: min out of range"
	errMaxOutOfRange = "error: max out of range"
	errMinGTMax      = "error: min cannot be greater than than max"

	errPortOutOfRange      = errors.New("error: port out of range")
	errAllPortsAssigned    = errors.New("error: all Ports assigned")
	errPortAlreadyAssigned = errors.New("error: port is already assigned")
)

func internalServerError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// A vendor provides an array of uint32 (Ports).
// Each array position > Ports[0] contains 0 or 1 depending
// on if the port has been assigned.
// Ports[0] holds the nominal next port to be assigned.
type vendor struct {
	Ports [limPort + 1]uint32 `json:"ports"`
}

// onIffOff atomically updates a port to on,
// if, and only if, the port was off.
// It returns a boolean of the operation's success.
func (v *vendor) onIffOff(port int) bool {
	return atomic.CompareAndSwapUint32(&v.Ports[port], 0, 1)
}

// off atomically updates a port to off,
// even if it was already off.
func (v *vendor) off(port int) {
	atomic.StoreUint32(&v.Ports[port], 0)
}

// loadNext loads Ports[0], the nominal
// next port to be assigned.
func (v *vendor) loadNext() uint32 {
	return atomic.LoadUint32(&v.Ports[0])
}

// updateNext updates Ports[0] with the nominal
// next port to be assigned.
func (v *vendor) updateNext(i uint32) {
	atomic.StoreUint32(&v.Ports[0], i)
}

// next assigns and returns the next available port.
// It will always initially try to assign the
// port value held in Ports[0], but if it fails
// will revert to a slower scan of all ports.
func (v *vendor) next() (int, error) {
	// Get "next" port.
	np := int(v.loadNext())
	if np > maxPort {
		return 0, errAllPortsAssigned
	}
	// Try assigning "next" port.
	if v.onIffOff(np) {
		v.updateNext(v.loadNext() + 1)
		return np, nil
	}
	// That failed, so scan all ports and attempt to assign.
	for n := range v.Ports[minPort:maxPort] {
		np = n + minPort
		if v.onIffOff(np) {
			v.updateNext(uint32(np) + 1)
			return np, nil
		}
	}
	v.updateNext(uint32(maxPort) + 1)
	return 0, errAllPortsAssigned
}

// assign assigns and returns a particular port.
// If port == autoPort it will assign the next
// available port.
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

// release un-assigns and returns a particular port.
// It will always return the port, even if it wasn't
// previously assigned.
func (v *vendor) release(port int) (int, error) {
	if port < minPort || port > maxPort {
		return 0, errPortOutOfRange
	}
	v.off(port)
	v.updateNext(uint32(port))
	return port, nil
}

// get handles GET operations.
func (v *vendor) get() (int, error) {
	port, err := v.assign(autoPort)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// post handles POST operations.
func (v *vendor) post(port int) (int, error) {
	port, err := v.assign(port)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// del handles DELETE operations.
func (v *vendor) del(port int) (int, error) {
	return v.release(port)
}

// ServeHTTP satisfies the http.Handler interface.
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

// Parse all flags, load the config, do sanity checks and initialise the port vendor
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
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
	var ports [limPort + 1]uint32
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
	np := int(v.Ports[0])
	switch {
	case np < minPort:
		v.Ports[0] = uint32(minPort)
	case np > maxPort+1:
		v.Ports[0] = uint32(maxPort) + 1
	}
}

func main() {
	// Create our server with conservative timeouts.
	server := &http.Server{
		Addr:         listen,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Setup signal notifiers in order to save data and cleanup.
	signalChan := make(chan os.Signal, 1)
	quit := make(chan struct{})
	defer close(signalChan)
	defer close(quit)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	go func() {
		for range signalChan {
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

	// Set the handler and run the server.
	http.Handle("/", v)
	log.Printf("Listening on %s\n", listen)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("ListenAndServe: %s\n", err)
		}
	}()
	log.Println("Press Ctrl-C to quit")

	// Wait for the cleanup to finish, then exit.
	<-quit
	log.Println("Exiting")
	os.Exit(0)
}
