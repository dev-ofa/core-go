package httpx

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"time"
)

// ResolveMode controls whether discovery should return only healthy instances.
type ResolveMode string

const (
	// ResolveModeHealthyOnly is the safe default for business calls.
	ResolveModeHealthyOnly ResolveMode = "healthy_only"
	// ResolveModeAll is intended for diagnostics or explicitly governed paths.
	ResolveModeAll ResolveMode = "all"
)

// HealthStatus describes an instance health state.
type HealthStatus string

const (
	// HealthStatusHealthy marks a callable instance.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusUnhealthy marks a non-callable instance.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusUnknown marks an instance without reliable health data.
	HealthStatusUnknown HealthStatus = "unknown"
)

// ServiceOptions configures service discovery for a request or client.
type ServiceOptions struct {
	// EnableDiscovery explicitly enables service discovery.
	EnableDiscovery bool
	// ServiceName overrides the logical service name parsed from URL host.
	ServiceName string
	// Namespace scopes the logical service name.
	Namespace string
	// PreferredZone expresses a soft topology preference.
	PreferredZone string
	// LabelSelector applies hard equality label filtering.
	LabelSelector map[string]string
	// PreferredLabelSelector applies soft equality label preference.
	PreferredLabelSelector map[string]string
	// ResolveMode controls health filtering. The default is healthy_only.
	ResolveMode ResolveMode
	// Resolver resolves logical services to instances.
	Resolver Resolver
	// Picker selects one instance from a discovery result.
	Picker InstancePicker
	// InstanceOverride bypasses resolver output for explicit debug scopes.
	InstanceOverride *Instance
}

// ResolveRequest is the caller-facing discovery request.
type ResolveRequest struct {
	ServiceName            string
	Namespace              string
	LabelSelector          map[string]string
	PreferredLabelSelector map[string]string
	PreferredZone          string
	ResolveMode            ResolveMode
	RequestID              string
	TraceID                string
}

// ResolveResponse is the caller-facing discovery response.
type ResolveResponse struct {
	ServiceName string
	Namespace   string
	Instances   []Instance
	ResolveTime time.Time
	Version     string
	CacheTTL    time.Duration
	Partial     bool
	Warnings    []string
}

// Instance describes a service instance returned by discovery.
type Instance struct {
	InstanceID   string
	Host         string
	Port         int
	Scheme       string
	HealthStatus HealthStatus
	Weight       int
	Zone         string
	Labels       map[string]string
}

// Resolver abstracts the service registry or sidecar discovery implementation.
type Resolver interface {
	Resolve(ctx context.Context, req ResolveRequest) (*ResolveResponse, error)
}

// ResolverFunc adapts a function into a Resolver.
type ResolverFunc func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error)

// Resolve implements Resolver.
func (f ResolverFunc) Resolve(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
	return f(ctx, req)
}

// InstancePicker selects one callable instance from a discovery response.
type InstancePicker interface {
	Pick(ctx context.Context, req ResolveRequest, resp *ResolveResponse) (*Instance, error)
}

// InstancePickerFunc adapts a function into an InstancePicker.
type InstancePickerFunc func(ctx context.Context, req ResolveRequest, resp *ResolveResponse) (*Instance, error)

// Pick implements InstancePicker.
func (f InstancePickerFunc) Pick(ctx context.Context, req ResolveRequest, resp *ResolveResponse) (*Instance, error) {
	return f(ctx, req, resp)
}

// RandomPicker selects instances with weight-aware random choice.
type RandomPicker struct{}

// Pick implements InstancePicker.
func (RandomPicker) Pick(_ context.Context, req ResolveRequest, resp *ResolveResponse) (*Instance, error) {
	if resp == nil || len(resp.Instances) == 0 {
		return nil, ErrNoHealthyInstance
	}
	candidates := make([]Instance, 0, len(resp.Instances))
	for _, inst := range resp.Instances {
		if req.ResolveMode != ResolveModeAll && inst.HealthStatus != "" && inst.HealthStatus != HealthStatusHealthy {
			continue
		}
		if req.PreferredZone != "" && inst.Zone != "" && inst.Zone != req.PreferredZone {
			continue
		}
		candidates = append(candidates, inst)
	}
	if len(candidates) == 0 && req.PreferredZone != "" {
		for _, inst := range resp.Instances {
			if req.ResolveMode != ResolveModeAll && inst.HealthStatus != "" && inst.HealthStatus != HealthStatusHealthy {
				continue
			}
			candidates = append(candidates, inst)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoHealthyInstance
	}
	total := 0
	for _, inst := range candidates {
		if inst.Weight > 0 {
			total += inst.Weight
		}
	}
	if total <= 0 {
		selected := candidates[rand.Intn(len(candidates))] // #nosec G404: load balancing does not require crypto randomness.
		return &selected, nil
	}
	pick := rand.Intn(total) // #nosec G404: load balancing does not require crypto randomness.
	for _, inst := range candidates {
		if inst.Weight <= 0 {
			continue
		}
		pick -= inst.Weight
		if pick < 0 {
			selected := inst
			return &selected, nil
		}
	}
	selected := candidates[len(candidates)-1]
	return &selected, nil
}

func resolveURL(ctx context.Context, original *url.URL, opt ServiceOptions, traceID string, requestID string) (*url.URL, string, error) {
	if !opt.EnableDiscovery {
		return original, "", nil
	}
	if opt.InstanceOverride != nil {
		u := rewriteURLToInstance(original, *opt.InstanceOverride)
		return u, original.Host, nil
	}
	if opt.Resolver == nil {
		return nil, "", ErrServiceDiscoveryDisabled
	}
	serviceName := opt.ServiceName
	namespace := opt.Namespace
	if serviceName == "" {
		serviceName, namespace = parseServiceIdentifier(original.Hostname(), namespace)
	}
	if serviceName == "" || namespace == "" {
		return nil, "", fmt.Errorf("service discovery requires service name and namespace")
	}
	mode := opt.ResolveMode
	if mode == "" {
		mode = ResolveModeHealthyOnly
	}
	req := ResolveRequest{
		ServiceName:            serviceName,
		Namespace:              namespace,
		LabelSelector:          opt.LabelSelector,
		PreferredLabelSelector: opt.PreferredLabelSelector,
		PreferredZone:          opt.PreferredZone,
		ResolveMode:            mode,
		RequestID:              requestID,
		TraceID:                traceID,
	}
	resp, err := opt.Resolver.Resolve(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("service resolve failed: %w", err)
	}
	picker := opt.Picker
	if picker == nil {
		picker = RandomPicker{}
	}
	inst, err := picker.Pick(ctx, req, resp)
	if err != nil {
		return nil, "", err
	}
	u := rewriteURLToInstance(original, *inst)
	return u, original.Host, nil
}

func parseServiceIdentifier(host string, namespace string) (string, string) {
	parts := stringsSplitNonEmpty(host, ".")
	if len(parts) >= 2 {
		if namespace == "" {
			namespace = parts[1]
		}
		return parts[0], namespace
	}
	return host, namespace
}

func rewriteURLToInstance(original *url.URL, inst Instance) *url.URL {
	u := *original
	if inst.Scheme != "" {
		u.Scheme = inst.Scheme
	}
	host := inst.Host
	if inst.Port > 0 {
		host = net.JoinHostPort(inst.Host, fmt.Sprintf("%d", inst.Port))
	}
	u.Host = host
	return &u
}

func stringsSplitNonEmpty(s string, sep string) []string {
	raw := strings.Split(s, sep)
	ret := make([]string, 0, len(raw))
	for _, item := range raw {
		if item != "" {
			ret = append(ret, item)
		}
	}
	return ret
}
