package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AccessKey         string
	Secret            string
	Session           string
	AccountID         string
	Region            string
	EC2Endpoint       string
	S3Endpoint        string
	IAMEndpoint       string
	AllowedHosts      []string
	AllowedAccounts   []string
	Target            string
	RunID             string
	Parallel          int
	Service           string
	Group             string
	Full              bool
	OutDir            string
	ConsoleAPIVersion string
}

func FromEnv() (Config, error) {
	c := Config{
		AccessKey:         os.Getenv("VWS_CONF_ACCESS_KEY_ID"),
		Secret:            os.Getenv("VWS_CONF_SECRET_ACCESS_KEY"),
		Session:           os.Getenv("VWS_CONF_SESSION_TOKEN"),
		AccountID:         os.Getenv("VWS_CONF_ACCOUNT_ID"),
		Region:            "vws-1",
		EC2Endpoint:       os.Getenv("VWS_CONF_EC2_ENDPOINT"),
		S3Endpoint:        os.Getenv("VWS_CONF_S3_ENDPOINT"),
		IAMEndpoint:       os.Getenv("VWS_CONF_IAM_ENDPOINT"),
		AllowedHosts:      split(os.Getenv("VWS_CONF_ALLOWED_HOSTS")),
		AllowedAccounts:   split(os.Getenv("VWS_CONF_ALLOWED_ACCOUNTS")),
		Target:            envDefault("VWS_CONF_TARGET", "local-fake"),
		RunID:             os.Getenv("VWS_CONF_RUN_ID"),
		OutDir:            envDefault("VWS_CONF_OUT", defaultOut()),
		ConsoleAPIVersion: envDefault("VWS_CONF_CONSOLE_API_VERSION", "unknown"),
		Service:           os.Getenv("VWS_CONF_SERVICE"),
		Group:             os.Getenv("VWS_CONF_GROUP"),
		Full:              os.Getenv("VWS_CONF_FULL") == "1",
	}
	n, err := ParseParallel(os.Getenv("VWS_CONF_PARALLEL"))
	if err != nil {
		return Config{}, err
	}
	c.Parallel = n
	switch c.Target {
	case "vws-test", "local-fake", "local-grpc":
	default:
		return Config{}, fmt.Errorf("VWS_CONF_TARGET %q is not a known target", c.Target)
	}
	if c.RunID != "" {
		if err := ValidateRunID(c.RunID); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

func ParseParallel(s string) (int, error) {
	if s == "" {
		return DefaultParallel, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("VWS_CONF_PARALLEL %q is not a positive integer", s)
	}
	return n, nil
}

func (c Config) Selected(service, group string) bool {
	if c.Full {
		return true
	}
	if c.Service == "" && c.Group == "" {
		return false
	}
	if c.Service != "" && c.Service != service {
		return false
	}
	if c.Group != "" && c.Group != group {
		return false
	}
	return true
}

func (c Config) CheckEndpoints() error {
	for _, ep := range []string{c.EC2Endpoint, c.S3Endpoint, c.IAMEndpoint} {
		if ep == "" {
			continue
		}
		if err := CheckEndpoint(ep, c.AllowedHosts); err != nil {
			return err
		}
	}
	if len(c.AllowedAccounts) == 0 {
		return fmt.Errorf("VWS_CONF_ALLOWED_ACCOUNTS is empty")
	}
	return nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func defaultOut() string {
	dir, err := os.Getwd()
	if err != nil {
		return "out"
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "out")
		}
		dir = filepath.Dir(dir)
	}
	return "out"
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
