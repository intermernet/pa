[![Build Status](https://drone.io/github.com/Intermernet/pw/status.png)](https://drone.io/github.com/Intermernet/pw/latest)

__pa - Port Authority__

pa is a web service for managing the assignment of TCP/IP ports on a host.

It presents a very simple REST API at http://0.0.0.0:3000/ which can be used to get the next available port, request a specific port, or delete a port assignment.

It will save any port assignments in a config file (`config.json`) when it exits, and reload them again on start-up.

Example usage:

    $ pa

    2014/12/09 15:37:29 Config file not found. Creating ./config.json
    2014/12/09 15:37:29 Listening on :3000

    $ curl http://localhost:3000/ # Request the next un-assigned port
    9000

    $ curl -XPOST http://localhost:3000/10080 # Request a specific port
    10080

    $ curl -XDELETE http://localhost:3000/10080 # Delete a port assignment
    10080

The port limits are currently hardcoded to the range of 9000 - 65535 (TCP/IP maximum port value).