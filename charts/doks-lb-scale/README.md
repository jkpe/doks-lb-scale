# DOKS Load Balancer Scale Controller Helm Chart

This Helm chart deploys the DOKS Load Balancer Scale Controller, a Kubernetes controller that automatically scales DigitalOcean load balancers based on metrics.

## Prerequisites

- Kubernetes 1.31+
- Helm 3.0+
- DigitalOcean API token (optional - only required if using DO API metrics)
- Prometheus (optional)

### Get Your DigitalOcean API Token (Optional)

**Note**: This step is only required if you plan to use DigitalOcean API metrics. If you're using Prometheus metrics only, you can skip this step.

- Create a DigitalOcean API token with least privileges:
  - Create a token with [Custom Scopes](https://docs.digitalocean.com/reference/api/create-personal-access-token/)
  - Grant only these scopes:
    - `monitoring:read`
- Create a Kubernetes secret with your DigitalOcean API token:

```bash
kubectl -n kube-system create secret generic doks-lb-scale-secret --from-literal=token=your-do-api-token-here
```

## Installation

### Quick Start

```bash
helm repo add doks-lb-scale https://your-repo-url
helm repo update
```

#### Install with existing DO API secret (for DO API metrics)

```bash
helm install doks-lb-scale ./charts/doks-lb-scale \
  --namespace kube-system \
  --set config.doApiTokenSecret="doks-lb-scale-secret"
```

#### Install with Prometheus URL only

```bash
helm install doks-lb-scale ./charts/doks-lb-scale \
  --namespace kube-system \
  --set config.prometheusUrl="http://kube-prometheus-stack-prometheus.kube-prometheus-stack.svc:9090"
```

#### Install with both

```bash
helm install doks-lb-scale ./charts/doks-lb-scale \
  --namespace kube-system \
  --set config.doApiTokenSecret="doks-lb-scale-secret" \
  --set config.prometheusUrl="http://kube-prometheus-stack-prometheus.kube-prometheus-stack.svc:9090"
```

## Configuration

For detailed configuration options, service annotations, and usage examples, please refer to the [main README](../README.md).

#### Verbose Logging

Enable verbose logging for debugging:

```yaml
config:
  verbose: true
```

## Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/digitalocean-labs/doks-lb-scale` |
| `image.tag` | Container image tag | `latest` |
| `deployment.replicas` | Number of replicas | `1` |
| `config.verbose` | Enable verbose logging | `false` |
| `config.doApiTokenSecret` | Name of existing secret containing DO API token | `""` |
| `config.prometheusUrl` | Prometheus server URL | `""` |
| `service.create` | Create service for monitoring | `false` |
| `serviceMonitor.create` | Create ServiceMonitor | `false` |
| `podDisruptionBudget.create` | Create PodDisruptionBudget | `false` |

## Upgrading

```bash
# Upgrade the release
helm upgrade doks-lb-scale doks-lb-scale/doks-lb-scale \
  --namespace kube-system \
  --values charts/doks-lb-scale/values.yaml
```

## Uninstalling

```bash
# Uninstall the release
helm uninstall doks-lb-scale --namespace kube-system
```

## Troubleshooting

### Check Controller Status

```bash
# Check pod status
kubectl get pods -n kube-system -l app.kubernetes.io/name=doks-lb-scale

# Check logs
kubectl logs -n kube-system -l app.kubernetes.io/name=doks-lb-scale

# Check events
kubectl get events -n kube-system --sort-by='.lastTimestamp'
```

### Common Issues

1. **Controller not scaling services**: Check that services have the required annotations
2. **API token errors**: Verify the DigitalOcean API token is valid and has appropriate permissions (only required for DO API metrics)
3. **Prometheus connection issues**: Ensure the Prometheus URL is accessible from the controller pod (only required for Prometheus metrics)
4. **Missing metric source**: Ensure either DO API token or Prometheus URL is configured based on your metric type

## Security

The chart includes security best practices:

- Non-root container execution
- Read-only root filesystem (in production)
- Dropped capabilities
- Pod security contexts
- RBAC with minimal required permissions

## Support

For issues and questions:

- GitHub Issues: [Repository Issues](https://github.com/digitalocean-labs/doks-lb-scale/issues)
- Documentation: [Project Documentation](https://github.com/digitalocean-labs/doks-lb-scale)
