[![Build Status](https://drone.io/github.com/Intermernet/pa/status.png)](https://drone.io/github.com/Intermernet/pa/latest)

__pa - Port Authority__

pa is a web service for managing the assignment of TCP/IP ports on a host.

__WARNING: This program is also an exercise in concurrent access to lock-free data structures. It probably has race conditions, despite `go build -race` not complaining, *USE AT OWN RISK!*__

It presents a very simple REST API at http://0.0.0.0:3000/ which can be used to get the next available port, request a specific port, or delete a port assignment.

It will save any port assignments in a config file (`pa.json`) when it exits, and reload them again on start-up.

Example usage:

    $ pa

    2015/06/23 16:49:26 Config file not found. Creating new config ./pa.json
    2015/06/23 16:49:26 Listening on :3000
    2015/06/23 16:49:26 Press Ctrl-C to quit

    $ curl http://localhost:3000/ # Request the next un-assigned port
    9000

    $ curl -XPOST http://localhost:3000/10080 # Request a specific port
    10080

    $ curl -XDELETE http://localhost:3000/10080 # Delete a port assignment
    10080

The port limits can be set with the `-min` and `-max` flags. They default to `-min=9000` and `-max=65535`.