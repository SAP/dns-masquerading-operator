/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"github.com/sap/dns-masquerading-operator/internal/controllers"
	"github.com/sap/dns-masquerading-operator/internal/coredns"
	"github.com/sap/go-generics/slices"
	// +kubebuilder:scaffold:imports
)

func TestOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator")
}

const controllerName = "masquerading-operator.cs.sap.com"
const corednsConfigMapNamespace = "kube-system"
const corednsConfigMapName = "coredns-custom"
const corednsConfigMapKey = "masquerading.override"
const corednsAddress = "127.0.0.1"
const corefileTemplate = `
.:{{ .listenPort }} {
    bind {{ .listenAddress }}
    errors
    log .
	debug
	prometheus
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        kubeconfig {{ .kubeconfigPath }}
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 1
    }
    forward . {{ .forwarderAddress }} {
        max_concurrent 1000
    }
    loop
    reload 2s 1s
    loadbalance
    import *.override
}
`

var testEnv *envtest.Environment
var cfg *rest.Config
var cli client.Client
var ctx context.Context
var cancel context.CancelFunc
var threads sync.WaitGroup
var tmpdir string
var namespace string
var resolver coredns.Resolver

var _ = BeforeSuite(func() {
	var err error

	By("initializing")
	ctrllog.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	tmpdir, err = os.MkdirTemp("", "")
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{"../../crds"},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			ValidatingWebhooks: []*admissionv1.ValidatingWebhookConfiguration{
				buildValidatingWebhookConfiguration(),
			},
		},
		// uncomment the following line to show control plane logs
		// AttachControlPlaneOutput: true,
	}
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	webhookInstallOptions := &testEnv.WebhookInstallOptions

	kubeconfigPath := fmt.Sprintf("%s/kubeconfig", tmpdir)
	err = clientcmd.WriteToFile(*kubeConfigFromRestConfig(cfg), kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	fmt.Printf("A temporary kubeconfig for the envtest environment can be found here: %s/kubeconfig\n", tmpdir)

	By("populating scheme")
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dnsv1alpha1.AddToScheme(scheme))

	By("initializing client")
	cli, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	By("starting coredns")
	corednsPath := os.Getenv("TEST_ASSET_COREDNS")
	if corednsPath == "" {
		corednsPath = os.Getenv("KUBEBUILDER_ASSETS") + "/coredns"
	}
	if corednsPath == "" {
		corednsPath = "/usr/local/kubebuilder/bin/coredns"
	}
	Expect(corednsPath).To(BeAnExistingFile())

	corednsPort, err := getFreePort(corednsAddress, true, true)
	Expect(err).NotTo(HaveOccurred())

	// note: we use the google resolver as forwarder, because using /etc/resolv.conf causes too many sporadic i/o timeout errors
	// TODO: find out why ...
	corednsForwarderAddress := "8.8.8.8"

	var corefileBuffer bytes.Buffer
	corefileData := map[string]string{
		"listenAddress":    corednsAddress,
		"listenPort":       strconv.Itoa(int(corednsPort)),
		"kubeconfigPath":   kubeconfigPath,
		"forwarderAddress": corednsForwarderAddress,
	}
	err = template.Must(template.New("corefile").Parse(corefileTemplate)).Execute(&corefileBuffer, corefileData)
	Expect(err).NotTo(HaveOccurred())
	corefile := corefileBuffer.Bytes()
	corefilePath := fmt.Sprintf("%s/Corefile", tmpdir)
	err = os.WriteFile(corefilePath, []byte(corefile), 0644)
	Expect(err).NotTo(HaveOccurred())

	threads.Add(1)
	go func() {
		defer threads.Done()
		defer GinkgoRecover()

		cmd := exec.CommandContext(ctx, corednsPath, "-conf", corefilePath)
		// uncomment the following lines to display coredns output
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr
		err = cmd.Run()
		if ctx.Err() == nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}()

	By("starting coredns configmap extractor")
	threads.Add(1)
	go func() {
		defer threads.Done()
		defer GinkgoRecover()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				configMap := &corev1.ConfigMap{}
				err := cli.Get(context.Background(), types.NamespacedName{Namespace: corednsConfigMapNamespace, Name: corednsConfigMapName}, configMap)
				if apierrors.IsNotFound(err) {
					continue
				}
				Expect(err).NotTo(HaveOccurred())

				newData, ok := configMap.Data[corednsConfigMapKey]
				if !ok {
					continue
				}

				path := fmt.Sprintf("%s/%s", tmpdir, corednsConfigMapKey)

				data, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					err = nil
					data = nil
				}
				Expect(err).NotTo(HaveOccurred())

				if data == nil || newData != string(data) {
					tmpPath := fmt.Sprintf("%s.tmp", path)
					err = os.WriteFile(tmpPath, []byte(newData), 0644)
					Expect(err).NotTo(HaveOccurred())
					err = os.Rename(tmpPath, path)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}
	}()

	By("initializing resolver")
	resolver = coredns.NewResolver(cli, cfg, false, coredns.Endpoint{Address: corednsAddress, Port: corednsPort})

	By("creating manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&dnsv1alpha1.MasqueradingRule{},
					&corev1.ConfigMap{},
				},
			},
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&controllers.MasqueradingRuleReconciler{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		Recorder:                    mgr.GetEventRecorderFor(controllerName),
		CorednsConfigMapNamespace:   corednsConfigMapNamespace,
		CorednsConfigMapName:        corednsConfigMapName,
		CorednsConfigMapKey:         corednsConfigMapKey,
		CorednsConfigMapUpdateDelay: 5 * time.Second,
		Resolver:                    resolver,
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&controllers.ServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&controllers.IngressReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&dnsv1alpha1.MasqueradingRule{}).SetupWebhookWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	By("starting dummy controller-manager")
	threads.Add(1)
	go func() {
		defer threads.Done()
		defer GinkgoRecover()
		// since there is no controller-manager in envtest, we have to confirm foreground deletion finalizer explicitly
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				masqueradingRuleList := &dnsv1alpha1.MasqueradingRuleList{}
				err := cli.List(context.Background(), masqueradingRuleList)
				Expect(err).NotTo(HaveOccurred())
				for _, masqueradingRule := range masqueradingRuleList.Items {
					if !masqueradingRule.DeletionTimestamp.IsZero() {
						if controllerutil.RemoveFinalizer(&masqueradingRule, metav1.FinalizerDeleteDependents) {
							err = cli.Update(context.Background(), &masqueradingRule)
							if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
								err = nil
							}
							Expect(err).NotTo(HaveOccurred())
						}
					}
				}
			}
		}
	}()

	By("starting manager")
	threads.Add(1)
	go func() {
		defer threads.Done()
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	By("waiting for operator to become ready")
	Eventually(func() error { return mgr.GetWebhookServer().StartedChecker()(nil) }, "10s", "100ms").Should(Succeed())

	By("create testing namespace")
	namespace, err = createNamespace()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	threads.Wait()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
	err = os.RemoveAll(tmpdir)
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Create masquerading rules", func() {
	var fromSpecific string
	var fromWildcard string
	var toDnsName string
	var toIpAddress string

	BeforeEach(func() {
		fromSpecific = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		fromWildcard = fmt.Sprintf("*.%s", randomString(8))
		toDnsName = "kubernetes.default.svc.cluster.local"
		toIpAddress = fmt.Sprintf("%d.%d.%d.%d", rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255))
	})

	It("should create a rule with specific source and IP target", func() {
		mr := &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: fromSpecific,
				To:   toIpAddress,
			},
		}
		err := cli.Create(ctx, mr)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(mr)
		validateRecord(mr.Spec.From, mr.Spec.To, 0)
	})

	It("should create a rule with specific source and DNS name target", func() {
		mr := &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: fromSpecific,
				To:   toDnsName,
			},
		}
		err := cli.Create(ctx, mr)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(mr)
		validateRecord(mr.Spec.From, mr.Spec.To, 0)
	})

	It("should reject a rule with wildcard source and IP target", func() {
		mr := &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: fromWildcard,
				To:   toIpAddress,
			},
		}
		err := cli.Create(ctx, mr)
		if !apierrors.IsForbidden(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should create a rule with wildcard source and DNS name target", func() {
		mr := &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: fromWildcard,
				To:   toDnsName,
			},
		}
		err := cli.Create(ctx, mr)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(mr)
		validateRecord(mr.Spec.From, mr.Spec.To, 0)
	})
})

var _ = Describe("Update masquerading rules", func() {
	var fromBefore string
	var fromAfter string
	var toBefore string
	var toAfter string
	var masqueradingRule *dnsv1alpha1.MasqueradingRule

	BeforeEach(func() {
		fromBefore = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		fromAfter = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		toBefore = "kubernetes.default.svc.cluster.local"
		toAfter = fmt.Sprintf("%d.%d.%d.%d", rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255))

		masqueradingRule = &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: fromBefore,
				To:   toBefore,
			},
		}
		err := cli.Create(ctx, masqueradingRule)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(masqueradingRule)
		validateRecord(masqueradingRule.Spec.From, masqueradingRule.Spec.To, 0)
	})

	It("should update the target of a rule", func() {
		masqueradingRule.Spec.To = toAfter
		err := cli.Update(ctx, masqueradingRule)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(masqueradingRule)
		validateRecord(masqueradingRule.Spec.From, masqueradingRule.Spec.To, 0)
	})

	It("should update the source of a rule", func() {
		masqueradingRule.Spec.From = fromAfter
		err := cli.Update(ctx, masqueradingRule)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(masqueradingRule)
		validateRecord(masqueradingRule.Spec.From, masqueradingRule.Spec.To, 0)
	})
})

var _ = Describe("Delete masquerading rules", func() {
	var from string
	var to string
	var masqueradingRule *dnsv1alpha1.MasqueradingRule

	BeforeEach(func() {
		from = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		to = fmt.Sprintf("%d.%d.%d.%d", rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255))

		masqueradingRule = &dnsv1alpha1.MasqueradingRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Spec: dnsv1alpha1.MasqueradingRuleSpec{
				From: from,
				To:   to,
			},
		}
		err := cli.Create(ctx, masqueradingRule)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleReady(masqueradingRule)
		validateRecord(masqueradingRule.Spec.From, masqueradingRule.Spec.To, 0)
	})

	It("should delete the rule", func() {
		err := cli.Delete(ctx, masqueradingRule)
		Expect(err).NotTo(HaveOccurred())
		waitForMasqueradingRuleGone(masqueradingRule)
		validateRecord(masqueradingRule.Spec.From, "", 20)
	})
})

var _ = Describe("Ingress tests", func() {
	var host1 string
	var host2 string
	var host3 string

	BeforeEach(func() {
		host1 = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		host2 = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
		host3 = fmt.Sprintf("%s.%s", randomString(10), randomString(5))
	})

	It("should maintain masquerading rules for the ingress", func() {
		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
				Annotations: map[string]string{
					"dns.cs.sap.com/masquerade-to": "kubernetes.default.svc.cluster.local",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: host1,
					},
					{
						Host: host2,
					},
				},
			},
		}

		err := cli.Create(ctx, ingress)
		Expect(err).NotTo(HaveOccurred())
		ensureMasqueradingRulesForIngress(ingress)

		err = cli.Get(ctx, types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}, ingress)
		Expect(err).NotTo(HaveOccurred())
		ingress.Spec.Rules[1].Host = host3
		err = cli.Update(ctx, ingress)
		Expect(err).NotTo(HaveOccurred())
		ensureMasqueradingRulesForIngress(ingress)

		err = cli.Get(ctx, types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}, ingress)
		Expect(err).NotTo(HaveOccurred())
		ingress.Spec.Rules = ingress.Spec.Rules[1:]
		err = cli.Update(ctx, ingress)
		Expect(err).NotTo(HaveOccurred())
		ensureMasqueradingRulesForIngress(ingress)

		err = cli.Get(ctx, types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}, ingress)
		Expect(err).NotTo(HaveOccurred())
		err = cli.Delete(ctx, ingress)
		Expect(err).NotTo(HaveOccurred())
		ingress.DeletionTimestamp = &[]metav1.Time{metav1.Now()}[0]
		ensureMasqueradingRulesForIngress(ingress)
	})

})

func waitForMasqueradingRuleReady(masqueradingRule *dnsv1alpha1.MasqueradingRule) {
	Eventually(func() error {
		if err := cli.Get(ctx, types.NamespacedName{Namespace: masqueradingRule.Namespace, Name: masqueradingRule.Name}, masqueradingRule); err != nil {
			return err
		}
		if masqueradingRule.Status.ObservedGeneration != masqueradingRule.Generation || masqueradingRule.Status.State != dnsv1alpha1.MasqueradingRuleStateReady {
			return fmt.Errorf("again")
		}
		return nil
	}, "120s", "500ms").Should(Succeed())
}

func waitForMasqueradingRuleGone(masqueradingRule *dnsv1alpha1.MasqueradingRule) {
	Eventually(func() error {
		err := cli.Get(ctx, types.NamespacedName{Namespace: masqueradingRule.Namespace, Name: masqueradingRule.Name}, masqueradingRule)
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err == nil {
			return fmt.Errorf("again")
		}
		return nil
	}, "120s", "500ms").Should(Succeed())
}

func validateRecord(from string, to string, timeout int) {
	from = regexp.MustCompile(`^\*(.*)$`).ReplaceAllString(from, `wildcard$1`)
	if timeout == 0 {
		active, err := resolver.CheckRecord(ctx, from, to)
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(active).To(BeTrue())
	} else {
		Eventually(func() error {
			active, err := resolver.CheckRecord(ctx, from, to)
			if err != nil {
				return err
			}
			if !active {
				return fmt.Errorf("again")
			}
			return nil
		}, timeout, "500ms")
	}
}

func ensureMasqueradingRulesForIngress(ingress *networkingv1.Ingress) {
	Eventually(func(g Gomega) {
		masqueradingRuleList := &dnsv1alpha1.MasqueradingRuleList{}
		err := cli.List(ctx, masqueradingRuleList, client.MatchingLabels{"dns.cs.sap.com/controller-uid": string(ingress.UID)})
		g.Expect(err).NotTo(HaveOccurred())
		if to, ok := ingress.Annotations["dns.cs.sap.com/masquerade-to"]; ok && ingress.DeletionTimestamp.IsZero() {
			g.Expect(masqueradingRuleList.Items).To(HaveLen(len(ingress.Spec.Rules)))
			for _, masqueradingRule := range masqueradingRuleList.Items {
				g.Expect(masqueradingRule.Spec.From).To(BeElementOf(slices.Collect(ingress.Spec.Rules, func(rule networkingv1.IngressRule) string { return rule.Host })))
				g.Expect(masqueradingRule.Spec.To).To(Equal(to))
			}
		} else {
			g.Expect(masqueradingRuleList.Items).To(BeEmpty())
		}
	}, "120s", "500ms").Should(Succeed())
}

// create namespace with a generated unique name
func createNamespace() (string, error) {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
	if err := cli.Create(ctx, namespace); err != nil {
		return "", err
	}
	return namespace.Name, nil
}

// assemble validatingwebhookconfiguration descriptor
func buildValidatingWebhookConfiguration() *admissionv1.ValidatingWebhookConfiguration {
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "validate-masqueradingrule",
		},
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:                    "validate-masqueradingrule.test.local",
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Path: &[]string{fmt.Sprintf("/validate-%s-%s-%s", strings.ReplaceAll(dnsv1alpha1.GroupVersion.Group, ".", "-"), dnsv1alpha1.GroupVersion.Version, "masqueradingrule")}[0],
				},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{
					admissionv1.Create,
					admissionv1.Update,
					admissionv1.Delete,
				},
				Rule: admissionv1.Rule{
					APIGroups:   []string{dnsv1alpha1.GroupVersion.Group},
					APIVersions: []string{dnsv1alpha1.GroupVersion.Version},
					Resources:   []string{"masqueradingrules"},
				},
			}},
			SideEffects: &[]admissionv1.SideEffectClass{admissionv1.SideEffectClassNone}[0],
		}},
	}
}

// convert rest.Config into kubeconfig
func kubeConfigFromRestConfig(restConfig *rest.Config) *clientcmdapi.Config {
	apiConfig := clientcmdapi.NewConfig()

	apiConfig.Clusters["envtest"] = clientcmdapi.NewCluster()
	cluster := apiConfig.Clusters["envtest"]
	cluster.Server = restConfig.Host
	cluster.CertificateAuthorityData = restConfig.CAData

	apiConfig.AuthInfos["envtest"] = clientcmdapi.NewAuthInfo()
	authInfo := apiConfig.AuthInfos["envtest"]
	authInfo.ClientKeyData = restConfig.KeyData
	authInfo.ClientCertificateData = restConfig.CertData

	apiConfig.Contexts["envtest"] = clientcmdapi.NewContext()
	context := apiConfig.Contexts["envtest"]
	context.Cluster = "envtest"
	context.AuthInfo = "envtest"

	apiConfig.CurrentContext = "envtest"

	return apiConfig
}

// get free port
func getFreePort(address string, tcp bool, udp bool) (uint16, error) {
	for i := 0; i < 10; i++ {
		port := 0
		if tcp {
			addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
			if err != nil {
				return 0, err
			}
			listener, err := net.ListenTCP("tcp", addr)
			listener.Close()
			if err != nil {
				continue
			}
			port = listener.Addr().(*net.TCPAddr).Port
		}
		if udp {
			addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(address, strconv.Itoa(port)))
			if err != nil {
				return 0, err
			}
			connection, err := net.ListenUDP("udp", addr)
			connection.Close()
			if err != nil {
				continue
			}
			port = connection.LocalAddr().(*net.UDPAddr).Port
		}
		if port != 0 {
			// TODO: the following cast is potentially unsafe (however no port numbers outside the 0-65535 range should occur)
			return uint16(port), nil
		}
	}
	return 0, fmt.Errorf("unable to find free port")
}

/*
// get system DNS resolver; not used, as we use 8.8.8.8 as forwarder
func getSystemResolver() (string, error) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return "", err
	}
	for _, server := range cfg.Servers {
		ip := net.ParseIP(server)
		if ip == nil || ip.To4() == nil {
			continue
		}
		return server, nil
	}
	return "", fmt.Errorf("no usable system resolver found")
}
*/

// create random lowercase character string of given length
func randomString(n int) string {
	charset := []byte("abcdefghijklmnopqrstuvwxyz")
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
