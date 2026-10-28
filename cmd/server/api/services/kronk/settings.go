package kronk

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"go.yaml.in/yaml/v2"
)

// Setting kinds.
const (
	KindString   = "string"
	KindBool     = "bool"
	KindInt      = "int"
	KindFloat    = "float"
	KindDuration = "duration"
	KindList     = "list"
	KindChoice   = "choice"
)

// Setting describes one server setting of the kms: section of the model
// config file. It is read from the server's config struct, so a new field
// shows up here without other changes.
type Setting struct {
	Key     string   // dot separated path under kms:, e.g. web.api-host
	Kind    string   // one of the Kind constants
	Default string   // the server's built-in default, empty when unset
	Help    string   // from the field's conf help tag
	Secret  bool     // masked in logs and help
	Values  []string // allowed values for KindChoice
}

// Settings lists the server settings in the order of the config struct.
func Settings() []Setting {
	cfg := newConfig()

	var out []Setting
	walkSettings(reflect.ValueOf(cfg), reflect.TypeOf(cfg), "", &out)
	return out
}

var (
	durationType = reflect.TypeFor[time.Duration]()
	modeType     = reflect.TypeFor[auth.Mode]()
)

func walkSettings(v reflect.Value, t reflect.Type, prefix string, out *[]Setting) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" || !f.IsExported() {
			continue
		}

		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		fv := v.Field(i)

		if f.Type.Kind() == reflect.Struct && f.Type != modeType {
			walkSettings(fv, f.Type, key, out)
			continue
		}

		s := Setting{Key: key}
		for opt := range strings.SplitSeq(f.Tag.Get("conf"), ",") {
			switch {
			case opt == "mask":
				s.Secret = true
			case strings.HasPrefix(opt, "help:"):
				s.Help = strings.TrimPrefix(opt, "help:")
			}
		}

		switch {
		case f.Type == durationType:
			s.Kind = KindDuration
			if d := fv.Interface().(time.Duration); d != 0 {
				s.Default = formatDuration(d)
			} else {
				s.Default = "0"
			}
		case f.Type == modeType:
			s.Kind = KindChoice
			s.Default = fv.Interface().(auth.Mode).String()
			s.Values = []string{auth.Open.String(), auth.Management.String(), auth.Authenticated.String(), auth.FullProtected.String()}
		case f.Type.Kind() == reflect.Bool:
			s.Kind = KindBool
			s.Default = strconv.FormatBool(fv.Bool())
		case f.Type.Kind() == reflect.Int:
			s.Kind = KindInt
			s.Default = strconv.FormatInt(fv.Int(), 10)
		case f.Type.Kind() == reflect.Float64:
			s.Kind = KindFloat
			s.Default = strconv.FormatFloat(fv.Float(), 'f', -1, 64)
		case f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String:
			s.Kind = KindList
			s.Default = strings.Join(fv.Interface().([]string), ",")
		default:
			s.Kind = KindString
			s.Default = fv.String()
		}

		*out = append(*out, s)
	}
}

// formatDuration drops the zero units Duration.String adds: 1h1m0s → 1h1m.
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// CheckKMS decodes a kms: section the way the server does, but strictly, so
// an unknown key or a bad value is an error before the file is written.
func CheckKMS(kms map[string]any) error {
	data, err := yaml.Marshal(kms)
	if err != nil {
		return err
	}

	cfg := newConfig()
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return fmt.Errorf("kms: %w", err)
	}

	return validateTimeoutConfig(cfg.Web.InferenceTimeout, cfg.Web.WriteTimeout)
}
