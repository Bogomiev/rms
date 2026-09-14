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
		"origin":         func(c *Config) { c.AppOrigins = []string{"https://app.test", "https://app.test/path"} },
		"missing origin": func(c *Config) { c.AppOrigins = nil; c.AppOrigin = "" },
		"attempt limit":  func(c *Config) { c.MaxLoginAttempts = 0 },
		"block duration": func(c *Config) { c.LoginBlockDuration = -1 },
		"env":            func(c *Config) { c.Env = "unknown" }, "port": func(c *Config) { c.Port = 65536 },
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

func TestAppOrigins(t *testing.T) {
	t.Setenv("CONFIG_PATH", "../../config/example.yaml")
	t.Setenv("RMS_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("RMS_APP_ORIGINS", "https://one.test,https://two.test:8443")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AppOrigins) != 2 || cfg.AppOrigins[1] != "https://two.test:8443" {
		t.Fatalf("unexpected origins: %v", cfg.AppOrigins)
	}
	cfg.AppOrigin = "https://legacy.test"
	if cfg.AllowedAppOrigins()[0] != "https://one.test" {
		t.Fatal("list must take precedence")
	}
	for _, invalid := range []string{"", "null", "*", "https://one.test/", "https://one.test?", "https://one.test#", "https://user@one.test", " https://one.test"} {
		cfg.AppOrigins = []string{"https://valid.test", invalid}
		if cfg.Validate() == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	cfg.AppOrigins = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("legacy fallback: %v", err)
	}
	cfg.Env = "prod"
	cfg.AppOrigins = []string{"https://one.test", "http://two.test"}
	if cfg.Validate() == nil {
		t.Fatal("production accepted HTTP origin")
	}
}
