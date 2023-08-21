/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package portforward

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward is a handle represents a port-forward connection.
type PortForward struct {
	config       *rest.Config
	localAddress string
	localPort    uint16
	namespace    string
	name         string
	port         uint16
	stopCh       chan struct{}
	mu           sync.Mutex
	started      bool
	stopped      bool
}

// Create new PortForward handle
func New(cfg *rest.Config, localAddress string, localPort uint16, namespace string, name string, port uint16) *PortForward {
	return &PortForward{
		config:       cfg,
		localAddress: localAddress,
		localPort:    localPort,
		namespace:    namespace,
		name:         name,
		port:         port,
		stopCh:       make(chan struct{}),
	}
}

// Start port-forwarding; blocks up to 10 seconds, until port-forward is ready, or an error or timeout occurred;
// Start() may be called only once (even after error); any further call will return an error.
func (pfw *PortForward) Start() error {
	pfw.mu.Lock()
	defer pfw.mu.Unlock()
	if pfw.started {
		return fmt.Errorf("error starting port forward %s:%d to %s/%s:%d: already started", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pfw.namespace, pfw.name)
	host := strings.TrimPrefix(pfw.config.Host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(pfw.config)
	if err != nil {
		return errors.Wrapf(err, "error starting port forward %s:%d to %s/%s:%d", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: host})

	readyCh := make(chan struct{})
	errorCh := make(chan error)
	fw, err := portforward.NewOnAddresses(dialer, []string{pfw.localAddress}, []string{fmt.Sprintf("%d:%d", pfw.localPort, pfw.port)}, pfw.stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return errors.Wrapf(err, "error starting port forward %s:%d to %s/%s:%d", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	}
	go func() {
		if err := fw.ForwardPorts(); err != nil {
			errorCh <- err
		}
	}()

	select {
	case <-readyCh:
		ports, err := fw.GetPorts()
		if err != nil {
			return errors.Wrapf(err, "error starting port forward %s:%d to %s/%s:%d", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
		}
		if len(ports) != 1 {
			return fmt.Errorf("error starting port forward %s:%d to %s/%s:%d: invalid port count returned (%d)", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port, len(ports))
		}
		if pfw.localPort != 0 && ports[0].Local != pfw.localPort {
			return fmt.Errorf("error starting port forward %s:%d to %s/%s:%d: invalid local port returned (%d)", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port, ports[0].Local)
		}
		if ports[0].Remote != pfw.port {
			return fmt.Errorf("error starting port forward %s:%d to %s/%s:%d: invalid remote port returned (%d)", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port, ports[0].Remote)
		}
		pfw.localPort = ports[0].Local
		pfw.started = true
		return nil
	case err := <-errorCh:
		close(pfw.stopCh)
		return errors.Wrapf(err, "error starting port forward %s:%d to %s/%s:%d", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	case <-time.After(10 * time.Second):
		close(pfw.stopCh)
		return fmt.Errorf("error starting port forward %s:%d to %s/%s:%d: timeout", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	}
}

// Stop port-forwarding; calling Stop() on a not yet started or already stopped handle has no effect.
func (pfw *PortForward) Stop() {
	pfw.mu.Lock()
	defer pfw.mu.Unlock()
	if !pfw.started || pfw.stopped {
		return
	}
	pfw.stopped = true
	close(pfw.stopCh)
}

func (pfw *PortForward) LocalAddress() string {
	return pfw.localAddress
}

func (pfw *PortForward) LocalPort() uint16 {
	return pfw.localPort
}
