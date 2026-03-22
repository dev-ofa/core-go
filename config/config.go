package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dev-ofa/core-go/trace/logging"
	"github.com/spf13/viper"
)

// Options defines the load order, naming rules, and validation hooks.
type Options struct {
	// DefaultConfigPath is the default config file path.
	DefaultConfigPath string
	// EnvPrefix is the environment variable prefix.
	EnvPrefix string
	// EnvSeparator is the hierarchy separator for env keys.
	EnvSeparator string
	// DeployEnvKey is the environment variable name for deployment profile, e.g. ENV.
	DeployEnvKey string
	// Args is the command line args to parse, defaulting to os.Args[1:].
	Args []string
	// RequiredKeys lists dot-path keys that must exist.
	RequiredKeys []string
	// SensitiveKeys lists dot-path keywords that must only come from env.
	SensitiveKeys []string
	// Strict enables strict unmarshal with unknown field rejection.
	Strict bool
	// LogEnabled controls whether load summary and hash are logged.
	LogEnabled bool
	// ValidateMap validates the merged config map before unmarshal.
	ValidateMap func(map[string]any) error
	// ValidateConfig validates the final config struct after unmarshal.
	ValidateConfig func(any) error
}

// Meta reports load sources, hash, and masked summary.
type Meta struct {
	// Sources lists the load sources in order.
	Sources []string
	// Hash is the stable hash of merged config.
	Hash string
	// Summary is the masked config summary for logging.
	Summary map[string]any
}

// Load merges config from default, env, local, and flags, then validates and decodes.
func Load[T any](opts Options) (T, Meta, error) {
	var zero T
	opts = withDefaults(opts)
	merged := map[string]any{}
	sourceMap := map[string]string{}
	var sources []string

	if m, ok, err := loadConfigIfExists(opts.DefaultConfigPath); err != nil {
		return zero, Meta{}, err
	} else if ok {
		merged = mergeMaps(merged, m)
		recordSources(sourceMap, m, "default", "")
		sources = append(sources, "default")
	}

	envMap := envToMap(opts.EnvPrefix, opts.EnvSeparator)
	if len(envMap) > 0 {
		merged = mergeMaps(merged, applyTypedOverrides(merged, envMap))
		recordSources(sourceMap, envMap, "env", "")
		sources = append(sources, "env")
	}

	if env := strings.TrimSpace(os.Getenv(opts.DeployEnvKey)); env != "" {
		baseDir := filepath.Dir(opts.DefaultConfigPath)
		envConfigPath := filepath.Join(baseDir, fmt.Sprintf("config.%s.yaml", strings.ToLower(env)))
		if m, ok, err := loadConfigIfExists(envConfigPath); err != nil {
			return zero, Meta{}, err
		} else if ok {
			merged = mergeMaps(merged, m)
			recordSources(sourceMap, m, "env-file", "")
			sources = append(sources, "env-file")
		}
	}

	flagMap := argsToMap(opts.Args)
	if len(flagMap) > 0 {
		merged = mergeMaps(merged, applyTypedOverrides(merged, flagMap))
		recordSources(sourceMap, flagMap, "flags", "")
		sources = append(sources, "flags")
	}

	if err := validateRequired(merged, opts.RequiredKeys); err != nil {
		return zero, Meta{}, err
	}
	if err := validateSensitiveSources(merged, sourceMap, opts.SensitiveKeys); err != nil {
		return zero, Meta{}, err
	}
	if opts.ValidateMap != nil {
		if err := opts.ValidateMap(merged); err != nil {
			return zero, Meta{}, err
		}
	}

	hash := hashMap(merged)
	summary := maskMap(merged, opts.SensitiveKeys)

	if opts.LogEnabled {
		logging.Infof("config loaded from %s", strings.Join(sources, ","))
		if b, err := json.Marshal(summary); err == nil {
			logging.Infof("config_hash=%s summary=%s", hash, string(b))
		} else {
			logging.Infof("config_hash=%s summary=%v", hash, summary)
		}
	}

	var out T
	cfg := viper.New()
	if err := cfg.MergeConfigMap(merged); err != nil {
		return zero, Meta{}, err
	}
	if opts.Strict {
		if err := cfg.UnmarshalExact(&out); err != nil {
			return zero, Meta{}, err
		}
	} else {
		if err := cfg.Unmarshal(&out); err != nil {
			return zero, Meta{}, err
		}
	}

	if opts.ValidateConfig != nil {
		if err := opts.ValidateConfig(out); err != nil {
			return zero, Meta{}, err
		}
	}

	return out, Meta{Sources: sources, Hash: hash, Summary: summary}, nil
}

// NewOptions returns best-practice defaults for config loading.
func NewOptions() Options {
	opts := Options{
		Strict:     true,
		LogEnabled: true,
	}
	return withDefaults(opts)
}

func withDefaults(opts Options) Options {
	if opts.DefaultConfigPath == "" {
		opts.DefaultConfigPath = "configs/config.default.yaml"
	}
	if opts.EnvPrefix == "" {
		opts.EnvPrefix = "APP"
	}
	if opts.EnvSeparator == "" {
		opts.EnvSeparator = "."
	}
	if opts.DeployEnvKey == "" {
		opts.DeployEnvKey = "ENV"
	}
	if opts.Args == nil {
		opts.Args = os.Args[1:]
	}
	if opts.SensitiveKeys == nil {
		opts.SensitiveKeys = []string{"password", "passwd", "secret", "token", "key", "uri"}
	}
	return opts
}

func loadConfigIfExists(path string) (map[string]any, bool, error) {
	if path == "" {
		return map[string]any{}, false, nil
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, false, nil
		}
		return map[string]any{}, false, err
	}
	m := normalizeMap(v.AllSettings())
	return m, true, nil
}

func envToMap(prefix, sep string) map[string]any {
	res := map[string]any{}
	if prefix == "" {
		return res
	}
	p := prefix + sep
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := parts[1]
		if !strings.HasPrefix(key, p) {
			continue
		}
		path := strings.TrimPrefix(key, p)
		if path == "" {
			continue
		}
		nodes := strings.Split(path, sep)
		for i := range nodes {
			nodes[i] = strings.ToLower(nodes[i])
		}
		setPath(res, nodes, val)
	}
	return res
}

func argsToMap(args []string) map[string]any {
	res := map[string]any{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		kv := strings.TrimPrefix(arg, "--")
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		if key == "" {
			continue
		}
		val := parts[1]
		nodes := strings.Split(key, ".")
		setPath(res, nodes, val)
	}
	return res
}

func normalizeMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range t {
			out[strings.ToLower(k)] = normalizeValue(v)
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for k, v := range t {
			out[strings.ToLower(fmt.Sprint(k))] = normalizeValue(v)
		}
		return out
	default:
		return map[string]any{}
	}
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any, map[any]any:
		return normalizeMap(t)
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, normalizeValue(item))
		}
		return out
	default:
		return v
	}
}

func mergeMaps(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if dv, ok := dst[k]; ok {
			if dm, ok := dv.(map[string]any); ok {
				if sm, ok := v.(map[string]any); ok {
					dst[k] = mergeMaps(dm, sm)
					continue
				}
			}
		}
		dst[k] = v
	}
	return dst
}

func setPath(m map[string]any, nodes []string, value any) {
	if len(nodes) == 0 {
		return
	}
	if len(nodes) == 1 {
		m[nodes[0]] = value
		return
	}
	next, ok := m[nodes[0]].(map[string]any)
	if !ok {
		next = map[string]any{}
		m[nodes[0]] = next
	}
	setPath(next, nodes[1:], value)
}

func getPath(m map[string]any, nodes []string) (any, bool) {
	if len(nodes) == 0 {
		return m, true
	}
	cur := m
	for i, n := range nodes {
		v, ok := cur[n]
		if !ok {
			return nil, false
		}
		if i == len(nodes)-1 {
			return v, true
		}
		nm, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = nm
	}
	return nil, false
}

func applyTypedOverrides(base, overrides map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range overrides {
		if vm, ok := v.(map[string]any); ok {
			var bm map[string]any
			if bv, ok := base[k].(map[string]any); ok {
				bm = bv
			}
			out[k] = applyTypedOverrides(bm, vm)
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = parseWithHint(base[k], s)
		} else {
			out[k] = v
		}
	}
	return out
}

func parseWithHint(hint any, s string) any {
	switch hint.(type) {
	case int:
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
	case int64:
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	case float64:
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	case bool:
		if v, err := strconv.ParseBool(s); err == nil {
			return v
		}
	}
	if v, err := strconv.ParseBool(s); err == nil {
		return v
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return s
}

func validateRequired(m map[string]any, keys []string) error {
	for _, key := range keys {
		nodes := strings.Split(strings.ToLower(key), ".")
		v, ok := getPath(m, nodes)
		if !ok || isEmpty(v) {
			return fmt.Errorf("missing %s", key)
		}
	}
	return nil
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func maskMap(m map[string]any, sensitive []string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		path := k
		out[k] = maskValue(path, v, sensitive)
	}
	return out
}

func maskValue(path string, v any, sensitive []string) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, vv := range t {
			next := path + "." + k
			out[k] = maskValue(next, vv, sensitive)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, maskValue(path, item, sensitive))
		}
		return out
	case string:
		if isSensitivePath(path, sensitive) {
			return "***"
		}
		if masked, ok := maskURI(t); ok {
			return masked
		}
		return t
	default:
		if isSensitivePath(path, sensitive) {
			return "***"
		}
		return v
	}
}

func isSensitivePath(path string, sensitive []string) bool {
	l := strings.ToLower(path)
	for _, s := range sensitive {
		if s == "" {
			continue
		}
		if strings.Contains(l, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

func maskURI(raw string) (string, bool) {
	if !strings.Contains(raw, "://") || !strings.Contains(raw, "@") {
		return "", false
	}
	schemeSplit := strings.SplitN(raw, "://", 2)
	if len(schemeSplit) != 2 {
		return "", false
	}
	userHost := schemeSplit[1]
	at := strings.Index(userHost, "@")
	if at < 0 {
		return "", false
	}
	userInfo := userHost[:at]
	rest := userHost[at+1:]
	if userInfo == "" {
		return "", false
	}
	userParts := strings.SplitN(userInfo, ":", 2)
	user := userParts[0]
	return schemeSplit[0] + "://" + user + ":***@" + rest, true
}

func hashMap(m map[string]any) string {
	flat := flattenMap(m, "")
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%v\n", k, flat[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func flattenMap(m map[string]any, prefix string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch t := v.(type) {
		case map[string]any:
			for fk, fv := range flattenMap(t, key) {
				out[fk] = fv
			}
		default:
			out[key] = v
		}
	}
	return out
}

func recordSources(dest map[string]string, m map[string]any, source, prefix string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if mv, ok := v.(map[string]any); ok {
			recordSources(dest, mv, source, key)
			continue
		}
		dest[key] = source
	}
}

func validateSensitiveSources(m map[string]any, sources map[string]string, sensitive []string) error {
	flat := flattenMap(m, "")
	for path, v := range flat {
		if !isSensitivePath(path, sensitive) {
			continue
		}
		if sources[path] == "env" {
			continue
		}
		if s, ok := v.(string); ok && isPlaceholder(s) {
			continue
		}
		return fmt.Errorf("sensitive config %s must come from env", path)
	}
	return nil
}

func isPlaceholder(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	if strings.Trim(t, "*") == "" {
		return true
	}
	if strings.Contains(t, "***") || strings.Contains(t, "******") {
		return true
	}
	switch strings.ToLower(t) {
	case "redacted", "<redacted>", "changeme", "replace_me", "placeholder":
		return true
	default:
		return false
	}
}
