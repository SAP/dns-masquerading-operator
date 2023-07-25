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
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PortForward struct {
	config       *rest.Config
	localAddress string
	localPort    int32
	namespace    string
	name         string
	port         int32
	stopCh       chan struct{}
}

func New(cfg *rest.Config, localAddress string, localPort int32, namespace string, name string, port int32) *PortForward {
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

func (pfw *PortForward) Start() error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pfw.namespace, pfw.name)
	host := strings.TrimPrefix(pfw.config.Host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(pfw.config)
	if err != nil {
		return err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: host})

	readyCh := make(chan struct{})
	errorCh := make(chan error)
	fw, err := portforward.NewOnAddresses(dialer, []string{pfw.localAddress}, []string{fmt.Sprintf("%d:%d", pfw.localPort, pfw.port)}, pfw.stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return err
	}
	go func() {
		if err := fw.ForwardPorts(); err != nil {
			errorCh <- err
		}
	}()

	select {
	case <-readyCh:
		return nil
	case err := <-errorCh:
		close(pfw.stopCh)
		return err
	case <-time.After(10 * time.Second):
		close(pfw.stopCh)
		return fmt.Errorf("error creating port forward %s:%d to %s/%s:%d (timeout)", pfw.localAddress, pfw.localPort, pfw.namespace, pfw.name, pfw.port)
	}
}

func (pfw *PortForward) Stop() {
	close(pfw.stopCh)
}
