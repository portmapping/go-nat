package nat

import (
	"context"
	"net"
	"time"

	"github.com/jackpal/go-nat-pmp"
)

var (
	_ NAT = (*natpmpNAT)(nil)
)

func discoverNATPMP(ctx context.Context) <-chan NAT {
	res := make(chan NAT, 1)

	ip, err := getDefaultGateway()
	if err != nil {
		return nil
	}

	go func() {
		defer close(res)
		// Unfortunately, we can't actually _stop_ the natpmp
		// library. However, we can at least close _our_ channel
		// and walk away.
		select {
		case client, ok := <-discoverNATPMPWithAddr(ip):
			if ok {
				res <- &natpmpNAT{c: client, gateway: ip}
			}
		case <-ctx.Done():
		}
	}()
	return res
}

func discoverNATPMPWithAddr(ip net.IP) <-chan *natpmp.Client {
	res := make(chan *natpmp.Client, 1)
	go func() {
		defer close(res)
		client := natpmp.NewClient(ip)
		_, err := client.GetExternalAddress()
		if err != nil {
			return
		}
		res <- client
	}()
	return res
}

type Port map[int]bool

type natpmpNAT struct {
	c       *natpmp.Client
	gateway net.IP
	extport int
	port    int
}

func (n *natpmpNAT) GetDeviceAddress() (addr net.IP, err error) {
	return n.gateway, nil
}

func (n *natpmpNAT) GetInternalAddress() (addr net.IP, err error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			switch x := addr.(type) {
			case *net.IPNet:
				if x.Contains(n.gateway) {
					return x.IP, nil
				}
			}
		}
	}

	return nil, ErrNoInternalAddress
}

func (n *natpmpNAT) GetExternalAddress() (addr net.IP, err error) {
	res, err := n.c.GetExternalAddress()
	if err != nil {
		return nil, err
	}

	d := res.ExternalIPAddress
	return net.IPv4(d[0], d[1], d[2], d[3]), nil
}

func (n *natpmpNAT) AddPortMapping(protocol string, internalPort int, description string, timeout time.Duration) (port int, err error) {
	timeoutInSeconds := int(timeout / time.Second)

	if n.port == internalPort && n.extport > 0 {
		_, err = n.c.AddPortMapping(protocol, n.port, n.extport, timeoutInSeconds)
		if err == nil {
			return n.extport, nil
		}
	}
	n.port = internalPort
	for i := 0; i < 3; i++ {
		externalPort := randomPort()
		_, err = n.c.AddPortMapping(protocol, n.port, externalPort, timeoutInSeconds)
		if err == nil {
			n.extport = externalPort
			return externalPort, nil
		}
	}

	return 0, err
}

func (n *natpmpNAT) DeletePortMapping(protocol string, internalPort int) (err error) {
	_, err = n.c.AddPortMapping(protocol, internalPort, n.extport, 0)
	return nil
}

func (n *natpmpNAT) Type() string {
	return "NAT-PMP"
}
