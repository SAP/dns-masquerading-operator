/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/sap/go-generics/pairs"
	"github.com/sap/go-generics/slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/dns-masquerading-operator/internal/dns"
	"github.com/sap/dns-masquerading-operator/internal/portforward"
)

type Endpoint struct {
	Namespace string
	Name      string
	Address   string
	Port      int32
}

// Check that specified host and expectedHost lead to the same DNS resolution result;
// the check is executed in parallel for all authoritative coredns pods found in the cluster.
func CheckRecord(ctx context.Context, c client.Client, cfg *rest.Config, host string, expectedHost string, inCluster bool) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	endpoints, err := discoverEndpoints(ctx, c)
	if err != nil {
		return false, err
	}

	results := make([]chan *pairs.Pair[bool, error], len(endpoints))
	for i := 0; i < len(endpoints); i++ {
		results[i] = make(chan *pairs.Pair[bool, error], 1)
		go func(i int) {
			if inCluster {
				log.V(1).Info("starting in-cluster lookup", "host", host, "serverAddress", endpoints[i].Address, "serverPort", endpoints[i].Port)
				var merr error
				addresses, err := dns.Lookup(host, endpoints[i].Address, endpoints[i].Port)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				expectedAddresses, err := dns.Lookup(expectedHost, endpoints[i].Address, endpoints[i].Port)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				results[i] <- pairs.New(merr == nil && len(addresses) > 0 && slices.Equal(addresses, expectedAddresses), merr)
			} else {
				log.V(1).Info("starting out-of-cluster lookup", "host", host, "serverNamespace", endpoints[i].Namespace, "serverName", endpoints[i].Name, "serverPort", endpoints[i].Port)
				localhost := "127.0.0.1"
				localport := int32(10000 + i)
				portforward := portforward.New(cfg, localhost, localport, endpoints[i].Namespace, endpoints[i].Name, endpoints[i].Port)
				if err := portforward.Start(); err != nil {
					results[i] <- pairs.New(false, err)
					return
				}
				defer portforward.Stop()
				var merr error
				addresses, err := dns.Lookup(host, localhost, localport)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				expectedAddresses, err := dns.Lookup(expectedHost, localhost, localport)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				results[i] <- pairs.New(merr == nil && len(addresses) > 0 && slices.Equal(addresses, expectedAddresses), merr)
			}
		}(i)
	}

	var merr error
	var active bool = true
	for _, result := range results {
		p := <-result
		if p.Y != nil {
			active = false
			merr = multierror.Append(merr, p.Y)
			continue
		}
		if !p.X {
			active = false
		}
	}

	return active, merr
}

// Discover (tcp) endpoints of all authoritative coredns pods found in the cluster.
func discoverEndpoints(ctx context.Context, c client.Client) ([]*Endpoint, error) {
	// TODO: parameterize things
	namespace := "kube-system" // same as corednsConfigMapNamespace, actually ...
	serviceName := "kube-dns"

	var portName string
	var targetPort int32

	service := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, service); err != nil {
		return nil, err
	}
	for _, servicePort := range service.Spec.Ports {
		if servicePort.Protocol == corev1.ProtocolTCP && servicePort.Port == 53 {
			portName = servicePort.Name
			targetPort = servicePort.TargetPort.IntVal
			break
		}
	}
	if portName == "" {
		return nil, fmt.Errorf("service %s does not have port tcp/53", serviceName)
	}

	var endpoints []*Endpoint

	if fakeEndpoints, ok := service.Annotations["testing.cs.sap.com/fake-endpoints"]; ok {
		// This is for testing only, to allow loopback addresses as endpoints (which is otherwise rejected by the endpoints api)
		for _, address := range strings.Split(fakeEndpoints, ",") {
			endpoint := &Endpoint{
				Address: address,
				Port:    targetPort,
			}
			endpoints = append(endpoints, endpoint)
		}
	} else {
		serviceEndpoints := &corev1.Endpoints{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, serviceEndpoints); err != nil {
			return nil, err
		}
		for _, subset := range serviceEndpoints.Subsets {
			var port int32
			for _, endpointPort := range subset.Ports {
				if endpointPort.Name == portName {
					port = endpointPort.Port
					break
				}
			}
			if port == 0 {
				continue
			}
			for _, address := range subset.Addresses {
				endpoint := &Endpoint{
					Namespace: address.TargetRef.Namespace,
					Name:      address.TargetRef.Name,
					Address:   address.IP,
					Port:      port,
				}
				endpoints = append(endpoints, endpoint)
			}
		}
	}

	// TODO: are endpoints unique by definition ?
	return endpoints, nil
}
