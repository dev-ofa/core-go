package resource

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	paramNameRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)
	schemeRegexp    = regexp.MustCompile(`^[a-z][a-z0-9_+.-]*$`)
)

// Parse parses an ofa-res resource identifier without doing network or disk IO.
func Parse(raw string) (Identifier, error) {
	if raw == "" {
		return Identifier{}, parseError(raw, "empty identifier")
	}
	if !strings.HasPrefix(raw, "ofa-res") {
		return Identifier{}, parseError(raw, "identifier must start with ofa-res")
	}
	meta, sourceURI, found := strings.Cut(raw, "#")
	if !found {
		return Identifier{}, parseError(raw, "missing source_uri separator")
	}
	if sourceURI == "" {
		return Identifier{}, parseError(raw, "empty source_uri")
	}
	if meta != "ofa-res" && !strings.HasPrefix(meta, "ofa-res?") {
		return Identifier{}, parseError(raw, "invalid metadata prefix")
	}

	params, err := parseParams(meta, raw)
	if err != nil {
		return Identifier{}, err
	}
	scheme, err := sourceScheme(sourceURI)
	if err != nil {
		return Identifier{}, &ParseError{Raw: raw, Err: err}
	}
	id := Identifier{
		Raw:       raw,
		Params:    params,
		SourceURI: sourceURI,
		Scheme:    scheme,
		AuthID:    params["auth_id"],
		MediaType: params["media_type"],
	}
	if err := validateIdentifierParams(params); err != nil {
		return Identifier{}, &ParseError{Raw: raw, Err: err}
	}
	return id, nil
}

func parseParams(meta string, raw string) (map[string]string, error) {
	params := map[string]string{}
	if meta == "ofa-res" {
		return params, nil
	}
	values, err := url.ParseQuery(strings.TrimPrefix(meta, "ofa-res?"))
	if err != nil {
		return nil, &ParseError{Raw: raw, Err: fmt.Errorf("parse params: %w", err)}
	}
	for key, vals := range values {
		if !paramNameRegexp.MatchString(key) {
			return nil, &ParseError{Raw: raw, Err: fmt.Errorf("invalid param name %q", key)}
		}
		if len(vals) != 1 {
			return nil, &ParseError{Raw: raw, Err: fmt.Errorf("duplicate param %q", key)}
		}
		params[key] = vals[0]
	}
	return params, nil
}

func sourceScheme(sourceURI string) (string, error) {
	colon := strings.IndexByte(sourceURI, ':')
	if colon <= 0 {
		return "", fmt.Errorf("source_uri scheme is required")
	}
	scheme := sourceURI[:colon]
	if !schemeRegexp.MatchString(scheme) {
		return "", fmt.Errorf("invalid source_uri scheme %q", scheme)
	}
	if strings.ToLower(scheme) != scheme {
		return "", fmt.Errorf("source_uri scheme must be lowercase")
	}
	return scheme, nil
}

func validateIdentifierParams(params map[string]string) error {
	filename := params["filename"]
	if filename != "" {
		if strings.ContainsAny(filename, `/\`+"\x00") || strings.Contains(filename, "..") {
			return fmt.Errorf("invalid filename")
		}
		for _, r := range filename {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("invalid filename")
			}
		}
	}
	return nil
}

func parseError(raw string, msg string) error {
	return &ParseError{Raw: raw, Err: fmt.Errorf("%s", msg)}
}
