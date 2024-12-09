/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package coredns

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/sap/go-generics/pairs"
	"github.com/sap/go-generics/slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/dns-masquerading-operator/internal/dnsutil"
	"github.com/sap/dns-masquerading-operator/internal/portforward"
)

// Resolver interface
type Resolver interface {
	// Check that the DNS resolution of host and expectedResult return the same address(es);
	// host must be a real DNS name, and must not be a wildcard name;
	// expectedResult may be a DNS name, an IP address, or empty, which means that the resolution of host
	// should not return any results, in order to make the check successful;
	// the boolean return value indicates success or failure of the check, the error return value
	// should be used to raise technical errors while performing the DNS resolution.
	CheckRecord(ctx context.Context, host string, expectedResult string) (bool, error)
}

// Endpoint representation for a namesever to be used be the resolver;
// Address and Port are mandatory; InCluster has to be set to true if the nameserver is runnning
// as a pod inside the cluster; in that case, Address and Port must point to that pod, and
// also Namespace and Name are required, referring to the according pod.
// If Incluster is false, Namespace and Name have no meaning and can be omitted.
type Endpoint struct {
	Address   string
	Port      uint16
	InCluster bool
	Namespace string
	Name      string
}

type resolver struct {
	client     client.Client
	restConfig *rest.Config
	inCluster  bool
	endpoints  []Endpoint
}

// Create new default resolver; the inCluster parameter has to be set to true if this operator is running inside the target cluster;
// if at least one endpoint is supplied, the specified endpoint(s) will be used for DNS queries;
// otherwise, the pod endpoints of the kube-system/kube-dns service will be used.
func NewResolver(client client.Client, restConfig *rest.Config, inCluster bool, endpoints ...Endpoint) Resolver {
	return &resolver{
		client:     client,
		restConfig: restConfig,
		inCluster:  inCluster,
		endpoints:  endpoints,
	}
}

// Check record (see Resolver interface)
func (r *resolver) CheckRecord(ctx context.Context, host string, expectedResult string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	endpoints := r.endpoints
	if len(endpoints) == 0 {
		clusterEndpoints, err := discoverEndpoints(ctx, r.client)
		if err != nil {
			return false, err
		}
		endpoints = clusterEndpoints
	}

	results := make([]chan *pairs.Pair[bool, error], len(endpoints))
	for i := 0; i < len(endpoints); i++ {
		results[i] = make(chan *pairs.Pair[bool, error], 1)
		go func(i int) {
			if endpoints[i].InCluster && !r.inCluster {
				log.V(1).Info("starting out-of-cluster lookup", "host", host, "serverNamespace", endpoints[i].Namespace, "serverName", endpoints[i].Name, "serverPort", endpoints[i].Port)
				localhost := "127.0.0.1"
				portforward := portforward.New(r.restConfig, localhost, 0, endpoints[i].Namespace, endpoints[i].Name, endpoints[i].Port)
				if err := portforward.Start(); err != nil {
					results[i] <- pairs.New(false, err)
					return
				}
				defer portforward.Stop()
				localport := portforward.LocalPort()
				var merr error
				addresses, err := dnsutil.Lookup(host, localhost, localport)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				if expectedResult == "" {
					results[i] <- pairs.New(merr == nil && len(addresses) == 0, merr)
				} else {
					expectedAddresses, err := dnsutil.Lookup(expectedResult, localhost, localport)
					if err != nil {
						merr = multierror.Append(merr, err)
					}
					results[i] <- pairs.New(merr == nil && len(addresses) > 0 && slices.Equal(addresses, expectedAddresses), merr)
				}
			} else {
				log.V(1).Info("starting lookup", "host", host, "serverAddress", endpoints[i].Address, "serverPort", endpoints[i].Port)
				var merr error
				addresses, err := dnsutil.Lookup(host, endpoints[i].Address, endpoints[i].Port)
				if err != nil {
					merr = multierror.Append(merr, err)
				}
				if expectedResult == "" {
					results[i] <- pairs.New(merr == nil && len(addresses) == 0, merr)
				} else {
					expectedAddresses, err := dnsutil.Lookup(expectedResult, endpoints[i].Address, endpoints[i].Port)
					if err != nil {
						merr = multierror.Append(merr, err)
					}
					results[i] <- pairs.New(merr == nil && len(addresses) > 0 && slices.Equal(addresses, expectedAddresses), merr)
				}
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

// discover endpoints of the kube-system/kube-dns service in target cluster
func discoverEndpoints(ctx context.Context, client client.Client) ([]Endpoint, error) {
	// TODO: parameterize things
	namespace := "kube-system" // same as corednsConfigMapNamespace, actually ...
	serviceName := "kube-dns"

	var portName string

	service := &corev1.Service{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, service); err != nil {
		return nil, err
	}
	for _, servicePort := range service.Spec.Ports {
		if servicePort.Protocol == corev1.ProtocolTCP && servicePort.Port == 53 {
			portName = servicePort.Name
			break
		}
	}
	if portName == "" {
		return nil, fmt.Errorf("service %s does not have port tcp/53", serviceName)
	}

	var endpoints []Endpoint

	serviceEndpoints := &corev1.Endpoints{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, serviceEndpoints); err != nil {
		return nil, err
	}

	for _, subset := range serviceEndpoints.Subsets {
		var port uint16
		for _, endpointPort := range subset.Ports {
			if endpointPort.Name == portName {
				// TODO: the following cast is potentially unsafe (however no port numbers outside the 0-65535 range should occur)
				port = uint16(endpointPort.Port)
				break
			}
		}
		if port == 0 {
			continue
		}
		for _, address := range subset.Addresses {
			if address.TargetRef.Kind != "Pod" {
				continue
			}
			endpoint := Endpoint{
				Address:   address.IP,
				Port:      port,
				InCluster: true,
				Namespace: address.TargetRef.Namespace,
				Name:      address.TargetRef.Name,
			}
			endpoints = append(endpoints, endpoint)
		}
	}

	// TODO: are endpoints unique by definition ?
	return endpoints, nil
}
