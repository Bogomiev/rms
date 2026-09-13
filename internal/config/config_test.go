package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidate(t *testing.T) {
	data, err := os.ReadFile("../../config/example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)
	t.Setenv("RMS_SIGNING_KEY", "01234567890123456789012345678901")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Config){
		"env": func(c *Config) { c.Env = "unknown" }, "port": func(c *Config) { c.Port = 65536 },
		"ttl": func(c *Config) { c.TokenTTL = 0 }, "refresh": func(c *Config) { c.RefreshTokenTTL = c.TokenTTL },
		"timeout": func(c *Config) { c.Timeout = -1 }, "pool": func(c *Config) { c.Db.MaxIdleConns = c.Db.MaxOpenConns + 1 },
		"tls": func(c *Config) { c.Db.SSLMode = "unknown" }, "certificate": func(c *Config) { c.Db.SSLCert = "cert" },
		"host": func(c *Config) { c.Db.Host = "" }, "connect timeout": func(c *Config) { c.Db.ConnectTimeout = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			copy := *cfg
			change(&copy)
			if copy.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
