package resource

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/dev-ofa/core-go/model/datax"
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
		return nil, &ParseError{Raw: raw, Err: datax.NewValidationError("parse params", nil, err)}
	}
	for key, vals := range values {
		if !paramNameRegexp.MatchString(key) {
			return nil, &ParseError{Raw: raw, Err: datax.NewValidationError(fmt.Sprintf("invalid param name %q", key), nil, nil)}
		}
		if len(vals) != 1 {
			return nil, &ParseError{Raw: raw, Err: datax.NewValidationError(fmt.Sprintf("duplicate param %q", key), nil, nil)}
		}
		params[key] = vals[0]
	}
	return params, nil
}

func sourceScheme(sourceURI string) (string, error) {
	colon := strings.IndexByte(sourceURI, ':')
	if colon <= 0 {
		return "", datax.NewValidationError("source_uri scheme is required", nil, nil)
	}
	scheme := sourceURI[:colon]
	if !schemeRegexp.MatchString(scheme) {
		return "", datax.NewValidationError(fmt.Sprintf("invalid source_uri scheme %q", scheme), nil, nil)
	}
	if strings.ToLower(scheme) != scheme {
		return "", datax.NewValidationError("source_uri scheme must be lowercase", nil, nil)
	}
	return scheme, nil
}

func validateIdentifierParams(params map[string]string) error {
	filename := params["filename"]
	if filename != "" {
		if strings.ContainsAny(filename, `/\`+"\x00") || strings.Contains(filename, "..") {
			return datax.NewValidationError("invalid filename", nil, nil)
		}
		for _, r := range filename {
			if r < 0x20 || r == 0x7f {
				return datax.NewValidationError("invalid filename", nil, nil)
			}
		}
	}
	return nil
}

func parseError(raw string, msg string) error {
	return &ParseError{Raw: raw, Err: datax.NewValidationError(msg, nil, nil)}
}
