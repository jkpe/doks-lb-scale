# doks-lb-scale

https://github.com/user-attachments/assets/9ab9f805-df87-49f1-98b4-edceb66a5b2f

A lightweight Kubernetes controller that automatically scales a DigitalOcean Load Balancer node size (size unit) based on Prometheus metrics from your ingress controller.

## How it works

- Watches `Service` objects of type `LoadBalancer` that include required annotations.
- Periodically fetches the configured Prometheus query.
  - `nginx_ingress_controller_requests` used as an example.
- Uses HTTP-style ingress metrics (e.g., total requests per second) to compute desired nodes.
- Computes the desired `size_unit` with hysteresis and min/max bounds and writes it back to the Service annotation.

DigitalOcean Cloud Controller Manager applies annotation changes to the actual Load Balancer.

## Prerequisites

- Install from the DigitalOcean Kubernetes Marketplace:
  - [Kubernetes Metrics Server](https://marketplace.digitalocean.com/apps/kubernetes-metrics-server)
  - [Kubernetes Monitoring Stack](https://marketplace.digitalocean.com/apps/kubernetes-monitoring-stack) (kube-prometheus-stack)
  - [Nginx Ingress Controller](https://marketplace.digitalocean.com/apps/nginx-ingress-controller) (optional, any ingress controller should work)

## Deploy

- Apply RBAC and Deployment:
 
```bash
kubectl apply -f config/rbac.yaml
kubectl apply -f config/deployment.yaml
```

Set the Prometheus URL via the `--prom-url` flag or `PROMETHEUS_URL` env var. The provided deployment sets `PROMETHEUS_URL` to `http://ingress-nginx-controller-metrics:9090` by default; adjust to your cluster.

## Required annotations

- `kubernetes.digitalocean.com/load-balancer-id`: the DO LB ID.
- `doks-lb-scale/metric`: the metric to use. Must be a Prometheus query prefixed with `promql:`.
- `doks-lb-scale/target-per-node`: REQUIRED: `req=<int>` (requests per second per node target)

Only HTTP/ingress metrics are supported.

Optional annotations:
- `doks-lb-scale/hysteresis-percent`: default `20`.
- `doks-lb-scale/min-nodes`: default `1`.
- `doks-lb-scale/max-nodes`: default `200`.
- `doks-lb-scale/scale-down-delay-minutes`: optional. If set to a positive integer, delays any scale-down by the specified number of minutes. The controller first sets a not-before timestamp and only applies the scale-down once that time has passed. Scaling up clears any pending delay.
- `service.beta.kubernetes.io/do-loadbalancer-size-unit`: set by controller.

## Example Service (HTTP requests)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "promql:sum(rate(nginx_ingress_controller_requests{ingress!=\"\",status!=\"\"}[1m]))"
    doks-lb-scale/target-per-node: "req=8000" # requests per node
    doks-lb-scale/hysteresis-percent: "20"
    doks-lb-scale/min-nodes: "1"
    doks-lb-scale/max-nodes: "50"
spec:
  type: LoadBalancer
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
```

## Example ingress-nginx Helm values

Use the following Helm values to deploy `ingress-nginx` with a `LoadBalancer` Service, metrics enabled for Prometheus scraping, and the required annotations for doks-lb-scale to manage the Load Balancer size:

```yaml
controller:
  replicaCount: 2
  service:
    type: LoadBalancer
    annotations:
      kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
      doks-lb-scale/metric: "promql:sum(rate(nginx_ingress_controller_requests{ingress!=\"\",status!=\"\"}[1m]))"
      doks-lb-scale/target-per-node: "req=8000"
      doks-lb-scale/hysteresis-percent: "20"
      doks-lb-scale/min-nodes: "1"
      doks-lb-scale/max-nodes: "50"
      doks-lb-scale/scale-down-delay-minutes: "10"
      service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
  metrics:
    enabled: true
    service:
      servicePort: "9090"
  podAnnotations:
    prometheus.io/port: "10254"
    prometheus.io/scrape: "true"
```

Pair this with the Prometheus-based example in the previous section (using `promql:sum(rate(nginx_ingress_controller_requests{ingress!="",status!=""}[1m]))`).

## Notes

- The controller performs a Prometheus instant query via `/api/v1/query?query=...` and uses the value from the first result.
- Up to date LB service annotations: [DigitalOcean CCM annotations](https://github.com/digitalocean/digitalocean-cloud-controller-manager/blob/master/docs/controllers/services/annotations.md)

## Hysteresis examples

`doks-lb-scale/hysteresis-percent` creates a no-change window around the current `size_unit`:

- lower = int(current × (1 − pct))
- upper = int(current × (1 + pct))

If desired is within [lower, upper], nothing changes.

Quick examples:
- current 10, pct 20% → window [8,12]; desired 12 = no change; 13 = scale up; 7 = scale down
- current 5, pct 10% → window [4,5]; desired 4 = no change; 6 = scale up; 3 = scale down
- current 1, pct 20% → window [0,1]; desired 1 = no change; ≥2 = scale up (min-nodes still applies)`

## Contact

If you wish to learn more about DigitalOcean's services, you are welcome to reach out to the sales team at sales@digitalocean.com. A global team of talented engineers will be happy to provide assistance.

## License

This Kubernetes controller, associated scripts and documentation in this project are released under the MIT License.
