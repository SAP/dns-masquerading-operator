/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package dnsutil

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sap/go-generics/slices"
)

// Lookup a DNS name on the specified DNS server, and return all IP addresses;
// returned slice of addresses will be nil if host was not found;
// if host is an IP address, it will be returned as such;
// err will be set for all other error situations.
func Lookup(host string, serverAddress string, serverPort uint16) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			// force network to "tcp"; not sure if this is a good idea ...
			return d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", serverAddress, serverPort))
		},
	}
	addresses, err := r.LookupHost(context.Background(), host)
	if err != nil {
		if err, ok := err.(*net.DNSError); ok && err.IsNotFound {
			return nil, nil
		}
		return nil, err
	}
	return slices.Sort(addresses), nil
}
