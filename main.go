/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"net"
	"os"
	"strconv"

	"github.com/pkg/errors"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	istioscheme "istio.io/client-go/pkg/clientset/versioned/scheme"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"github.com/sap/dns-masquerading-operator/internal/controllers"
	"github.com/sap/dns-masquerading-operator/internal/coredns"
	//+kubebuilder:scaffold:imports
)

const (
	LeaderElectionID = "masquerading-operator.cs.sap.com"
)

const (
	controllerName         = "masquerading-operator.cs.sap.com"
	inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(istioscheme.AddToScheme(scheme))

	utilruntime.Must(dnsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeAddr string
	var webhookAddr string
	var webhookCertDir string
	var enableLeaderElection bool
	var leaderElectionNamespace string
	var corednsConfigMapNamespace string
	var corednsConfigMapName string
	var corednsConfigMapKey string
	var enableServiceController bool
	var enableIngressController bool
	var enableIstioGatewayController bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&webhookAddr, "webhook-bind-address", ":9443", "The address the webhook endpoint binds to.")
	flag.StringVar(&webhookCertDir, "webhook-tls-directory", "", "The directory containing tls server key and certificate, as tls.key and tls.crt; defaults to $TMPDIR/k8s-webhook-server/serving-certs")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "", "The namespace to use for the leader election lock; defaults to controller namespace when running in-cluster.")
	flag.StringVar(&corednsConfigMapNamespace, "coredns-configmap-namespace", "kube-system", "The namespace of the coredns extension configmap where this controller stores the rewrite rules")
	flag.StringVar(&corednsConfigMapName, "coredns-configmap-name", "coredns-custom", "The name of the coredns extension configmap where this controller stores the rewrite rules")
	flag.StringVar(&corednsConfigMapKey, "coredns-configmap-key", "masquerading-operator.override", "The key in the coredns extension configmap where this controller stores the rewrite rules")
	flag.BoolVar(&enableServiceController, "enable-service-controller", false, "Whether to generate masquerading rules based on services as a source")
	flag.BoolVar(&enableIngressController, "enable-ingress-controller", false, "Whether to generate masquerading rules based on ingresses as a source")
	flag.BoolVar(&enableIstioGatewayController, "enable-istiogateway-controller", false, "Whether to generate masquerading rules based on istio gateways as a source")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	inCluster, inClusterNamespace, err := checkInCluster()
	if err != nil {
		setupLog.Error(err, "unable to check if running in cluster")
		os.Exit(1)
	}

	webhookHost, webhookPort, err := parseAddress(webhookAddr)
	if err != nil {
		setupLog.Error(err, "unable to parse webhook bind address")
		os.Exit(1)
	}

	if enableLeaderElection && leaderElectionNamespace == "" {
		if inCluster {
			leaderElectionNamespace = inClusterNamespace
		} else {
			setupLog.Error(nil, "missing command line parameter", "flag", "--leader-election-namespace")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&dnsv1alpha1.MasqueradingRule{},
					&corev1.ConfigMap{},
				},
			},
		},
		LeaderElection:                enableLeaderElection,
		LeaderElectionNamespace:       leaderElectionNamespace,
		LeaderElectionID:              LeaderElectionID,
		LeaderElectionReleaseOnCancel: true,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookHost,
			Port:    webhookPort,
			CertDir: webhookCertDir,
		}),
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if enableServiceController {
		if err = (&controllers.ServiceReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Service")
			os.Exit(1)
		}
	}

	if enableIngressController {
		if err = (&controllers.IngressReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Ingress")
			os.Exit(1)
		}
	}

	if enableIstioGatewayController {
		if err = (&controllers.GatewayReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Gateway")
			os.Exit(1)
		}
	}

	if err = (&controllers.MasqueradingRuleReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		Recorder:                  mgr.GetEventRecorderFor(controllerName),
		CorednsConfigMapNamespace: corednsConfigMapNamespace,
		CorednsConfigMapName:      corednsConfigMapName,
		CorednsConfigMapKey:       corednsConfigMapKey,
		Resolver:                  coredns.NewResolver(mgr.GetClient(), mgr.GetConfig(), inCluster),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MasqueradingRule")
		os.Exit(1)
	}
	if err = (&dnsv1alpha1.MasqueradingRule{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MasqueradingRule")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseAddress(address string) (string, int, error) {
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		return "", -1, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", -1, err
	}
	return host, port, nil
}

func checkInCluster() (bool, string, error) {
	_, err := os.Stat(inClusterNamespacePath)
	if os.IsNotExist(err) {
		return false, "", nil
	} else if err != nil {
		return false, "", errors.Wrap(err, "error checking namespace file")
	}

	namespace, err := os.ReadFile(inClusterNamespacePath)
	if err != nil {
		return false, "", errors.Wrap(err, "error reading namespace file")
	}

	return true, string(namespace), nil
}
