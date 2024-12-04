# DNS Masquerading For Kubernetes Clusters

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/dns-masquerading-operator)](https://api.reuse.software/info/github.com/SAP/dns-masquerading-operator)

## About this project

Some Kubernetes providers offer a mechanism to enhance configuration of the cluster-internal DNS (which is usually coredns).
For example:

- Azure: [https://github.com/MicrosoftDocs/azure-docs/blob/main/articles/aks/coredns-custom.md](https://github.com/MicrosoftDocs/azure-docs/blob/main/articles/aks/coredns-custom.md)
- Gardener [https://github.com/gardener/gardener/blob/master/docs/usage/custom-dns-config.md](https://github.com/gardener/gardener/blob/master/docs/usage/custom-dns-config.md).

A common usecase of this feature is to add DNS masquerading rules to the cluster DNS, leveraging the coredns [rewrite](https://coredns.io/plugins/rewrite/) and [hosts](https://coredns.io/plugins/hosts/) modules.

From a technical perspective, the extension usually happens by defining a key in some config map, containing a coredns configuration snippet, which will be included into the cluster's main coredns configuration. The operator provided by this repository allows to declaratively manage such a config map key in a declarative way.

The masquerading rules are maintained through the custom resource `MasqueradingRule` (group `dns.cs.sap.com`), such as:

```yaml
apiVersion: dns.cs.sap.com/v1alpha1
kind: MasqueradingRule
metadata:
  namespace: my-namespace
  name: my-rule
spec:
  from: <hostname to be rewritten>
  to: <target hostname or IP address>
```

As a result, the cluster's coredns will be configured to answer DNS lookups for ` <hostname to be rewritten>`
- with the IP address directly specified in the `to` field, or
- with the IP address(es) that the `<target hostname>` resolves to.

A wildcard DNS name (first DNS label being '*') is allowed to be specified as `from`, if `to` is a DNS name too.

A special (but important) usecase is to rewrite external DNS names of services, ingresses or istio gateways to some cluster-internal endpoint.
To support this usecase, the operator optionally allows to automatically maintain according `MasqueradingRule` instances by annotating services, ingresses, or istio gateways, such as:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    dns.cs.sap.com/masquerade-to: nginx-ingress-corporate-controller.cs-system.svc.cluster.local
  name: test
  namespace: cs-system
spec:
  ...
```

For ingresses and istio gateways, the masqueraded hostnames are derived from the resource's spec, in the obvious sense.
For services, the masqueraded hostnames are determined from one or a combination of the following annotations:
- `dns.cs.sap.com/masquerade-from`
- `external-dns.alpha.kubernetes.io/hostname`
- `dns.gardener.cloud/dnsnames`

If `dns.cs.sap.com/masquerade-from` is set, then the annotation `dns.cs.sap.com/masquerade-to` is optional; if missing it will be defaulted with
the in-cluster address of the service.

## Requirements and Setup

The recommended deployment method is to use the [Helm chart](https://github.com/sap/dns-masquerading-operator-helm):

```bash
helm upgrade -i dns-masquerading-operator oci://ghcr.io/sap/dns-masquerading-operator-helm/dns-masquerading-operator
```

## Documentation
 
The API reference is here: [https://pkg.go.dev/github.com/sap/dns-masquerading-operator](https://pkg.go.dev/github.com/sap/dns-masquerading-operator).

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/dns-masquerading-operator/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/dns-masquerading-operator).
