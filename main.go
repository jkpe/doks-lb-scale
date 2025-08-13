package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	AnnotationLoadBalancerID = "kubernetes.digitalocean.com/load-balancer-id"
	AnnotationSizeUnit       = "service.beta.kubernetes.io/do-loadbalancer-size-unit"
	AnnotationMetric         = "doks-lb-scale/metric"
	AnnotationTargetPerNode  = "doks-lb-scale/target-per-node"    // required: "req=INT" or "nlb=INT"
	AnnotationHysteresisPct  = "doks-lb-scale/hysteresis-percent" // default 20
	AnnotationMinNodes       = "doks-lb-scale/min-nodes"          // default 1
	AnnotationMaxNodes       = "doks-lb-scale/max-nodes"          // default 200
)

var (
	scheme   = runtime.NewScheme()
	setupLog = log.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

type MetricsClient interface {
	GetValue(ctx context.Context, lbID string, metric string) (float64, error)
}

type DOClient struct {
	APIToken string
}

// Implementation in do_client.go

type Reconciler struct {
	client.Client
	Metrics MetricsClient
	Verbose bool
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if r.Verbose {
		klog.InfoS("reconcile start", "service", req.NamespacedName)
	}
	var svc corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	ann := svc.GetAnnotations()
	if ann == nil {
		return reconcile.Result{}, nil
	}
	lbID := ann[AnnotationLoadBalancerID]
	metric := ann[AnnotationMetric]
	rawTarget := ann[AnnotationTargetPerNode]
	if lbID == "" || metric == "" || strings.TrimSpace(rawTarget) == "" {
		return reconcile.Result{}, nil
	}

	category := categoryFromMetric(metric) // "req" or "nlb"
	if category == "nlb" && !isThroughputMetric(metric) {
		klog.InfoS("skipping scale: NLB requires throughput metric", "metric", metric, "service", req.NamespacedName)
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}

	targetPerNode, ok := resolveStrictTargetPerNode(rawTarget, category)
	if !ok {
		klog.InfoS("skipping scale: target-per-node must specify exactly one of req= or nlb= and match metric category", "target", rawTarget, "category", category, "service", req.NamespacedName)
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}

	if r.Verbose {
		klog.InfoS("fetching metrics", "lbID", lbID, "metric", metric, "service", req.NamespacedName)
	}
	value, err := r.Metrics.GetValue(ctx, lbID, metric)
	if err != nil {
		klog.ErrorS(err, "failed to fetch metrics", "lbID", lbID, "metric", metric)
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}
	if r.Verbose {
		klog.InfoS("metrics value", "value", value, "service", req.NamespacedName)
	}

	desired := computeDesiredNodes(ann, targetPerNode, value)
	if desired == 0 {
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}

	current := parseIntDefault(ann[AnnotationSizeUnit], 1)
	if r.Verbose {
		klog.InfoS("computed desired nodes", "current", current, "desired", desired, "service", req.NamespacedName)
	}
	if current == desired {
		if r.Verbose {
			klog.InfoS("no change in size-unit", "current", current, "service", req.NamespacedName)
		}
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Hysteresis
	hysteresis := parseIntDefault(ann[AnnotationHysteresisPct], 20)
	lower := int(float64(current) * (1 - float64(hysteresis)/100.0))
	upper := int(float64(current) * (1 + float64(hysteresis)/100.0))
	if desired >= lower && desired <= upper {
		if r.Verbose {
			klog.InfoS("within hysteresis window, skipping update", "lower", lower, "upper", upper, "desired", desired, "current", current, "service", req.NamespacedName)
		}
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	if r.Verbose {
		klog.InfoS("updating service size-unit", "from", current, "to", desired, "service", req.NamespacedName)
	}
	svc.Annotations[AnnotationSizeUnit] = itoa(desired)
	if err := r.Update(ctx, &svc); err != nil {
		klog.ErrorS(err, "failed to update service annotation")
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}
	if r.Verbose {
		klog.InfoS("service annotation updated", "size-unit", desired, "service", req.NamespacedName)
	}
	return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
}

func computeDesiredNodes(ann map[string]string, targetPerNode int, metricValue float64) int {
	minNodes := clamp(parseIntDefault(ann[AnnotationMinNodes], 1), 1, 200)
	maxNodes := clamp(parseIntDefault(ann[AnnotationMaxNodes], 200), 1, 200)
	if targetPerNode <= 0 {
		return minNodes
	}
	required := int((metricValue + float64(targetPerNode-1)) / float64(targetPerNode))
	if required < minNodes {
		return minNodes
	}
	if required > maxNodes {
		return maxNodes
	}
	return required
}

// resolveStrictTargetPerNode enforces required keyed single-entry config.
// raw must be either "req=INT" or "nlb=INT"; returns value only if key matches category.
func resolveStrictTargetPerNode(raw string, category string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 1 {
		return 0, false
	}
	kv := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
	if len(kv) != 2 {
		return 0, false
	}
	key := strings.ToLower(strings.TrimSpace(kv[0]))
	val := strings.TrimSpace(kv[1])
	if key != "req" && key != "nlb" {
		return 0, false
	}
	if key != category {
		return 0, false
	}
	return parseIntDefault(val, 0), true
}

// categoryFromMetric maps metrics to logical categories used for target-per-node keys.
// Returns "nlb" for NLB throughput metrics, otherwise "req".
func categoryFromMetric(metric string) string {
	m := strings.ToLower(metric)
	if isThroughputMetric(m) {
		return "nlb"
	}
	return "req"
}

func isThroughputMetric(metric string) bool {
	m := strings.ToLower(metric)
	return strings.Contains(m, "nlb_tcp_network_throughput") || strings.Contains(m, "nlb_udp_network_throughput")
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return def
		}
		v = v*10 + int(ch-'0')
	}
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		d := byte(v % 10)
		buf = append([]byte{'0' + d}, buf...)
		v /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func main() {
	var doToken string
	var addr string
	var verboseFlag bool
	flag.StringVar(&doToken, "do-token", os.Getenv("DIGITALOCEAN_TOKEN"), "DigitalOcean API token")
	flag.StringVar(&addr, "bind", ":8080", "healthz bind address")
	flag.BoolVar(&verboseFlag, "verbose", false, "enable verbose logging")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: "0"},
		HealthProbeBindAddress: addr,
		LeaderElection:         true,
		LeaderElectionID:       "doks-lb-scale-controller",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	verbose := verboseFlag || envBool("DOKS_LB_SCALE_VERBOSE")
	reconciler := &Reconciler{Client: mgr.GetClient(), Metrics: &DOClient{APIToken: doToken}, Verbose: verbose}
	if err := builder.ControllerManagedBy(mgr).
		For(&corev1.Service{}, builder.WithPredicates(serviceHasLBAnnotations())).
		Complete(reconciler); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

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

func serviceHasLBAnnotations() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return filterService(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return filterService(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return filterService(e.Object) },
	}
}

func filterService(obj client.Object) bool {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return false
	}
	ann := svc.GetAnnotations()
	if ann == nil {
		return false
	}
	if ann[AnnotationLoadBalancerID] == "" {
		return false
	}
	if strings.TrimSpace(ann[AnnotationMetric]) == "" {
		return false
	}
	if strings.TrimSpace(ann[AnnotationTargetPerNode]) == "" {
		return false
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	return true
}

func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
