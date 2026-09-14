package config

import (
	"fmt"
	"net/url"
	"os"
	"rms/internal/scheduler"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	AppOrigins         []string         `yaml:"app_origins" env:"RMS_APP_ORIGINS" env-separator:","`
	AppOrigin          string           `yaml:"app_origin" env:"RMS_APP_ORIGIN"`
	MaxLoginAttempts   int              `yaml:"max_login_attempts" env:"RMS_MAX_LOGIN_ATTEMPTS" env-default:"5"`
	LoginBlockDuration time.Duration    `yaml:"login_block_duration" env:"RMS_LOGIN_BLOCK_DURATION" env-default:"15m"`
	FirstAdminPwd      string           `yaml:"first_admin_pwd"`
	Scheduler          scheduler.Config `yaml:"scheduler"`
	Env                string           `yaml:"env" env-default:"local"`
	SigningKey         string           `yaml:"signing_key" env:"RMS_SIGNING_KEY"`
	Db                 DbSetting        `yaml:"db"`
	HTTPServer         `yaml:"http_server"`
	TokenTTL           time.Duration `yaml:"token_TTL" env-default:"15m"`
	RefreshTokenTTL    time.Duration `yaml:"refresh_token_TTL" env-default:"2160h"`
}

type HTTPServer struct {
	Port        int           `yaml:"port" env-default:"8080"`
	Timeout     time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

type DbSetting struct {
	SSLMode         string        `yaml:"sslmode" env-default:"disable"`
	SSLRootCert     string        `yaml:"sslrootcert"`
	SSLCert         string        `yaml:"sslcert"`
	SSLKey          string        `yaml:"sslkey"`
	ConnectTimeout  time.Duration `yaml:"connect_timeout" env-default:"5s"`
	MaxOpenConns    int           `yaml:"max_open_conns" env-default:"10"`
	MaxIdleConns    int           `yaml:"max_idle_conns" env-default:"5"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env-default:"30m"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" env-default:"5m"`

	Host     string `yaml:"host"`
	Port     int    `yaml:"port" env-default:"5432"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DbName   string `yaml:"dbname"`
}

func Load() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		return nil, fmt.Errorf("CONFIG_PATH is not set")
	}
	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// AllowedAppOrigins prefers the list; app_origin remains a legacy fallback.
func (c Config) AllowedAppOrigins() []string {
	if len(c.AppOrigins) > 0 {
		return c.AppOrigins
	}
	if c.AppOrigin != "" {
		return []string{c.AppOrigin}
	}
	return nil
}

func (c Config) Validate() error {
	origins := c.AllowedAppOrigins()
	if len(origins) == 0 {
		return fmt.Errorf("app_origins must contain at least one origin")
	}
	for _, value := range origins {
		origin, err := url.Parse(value)
		if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery || strings.ContainsAny(value, "?#") || (origin.Scheme != "http" && origin.Scheme != "https") {
			return fmt.Errorf("app_origins entries must be an HTTP(S) origin without a path")
		}
		if c.Env == "prod" && origin.Scheme != "https" {
			return fmt.Errorf("app_origins entries must use HTTPS in prod")
		}
	}
	if c.MaxLoginAttempts < 1 || c.LoginBlockDuration <= 0 {
		return fmt.Errorf("login attempt limit and block duration must be positive")
	}
	if c.Env != "local" && c.Env != "dev" && c.Env != "prod" {
		return fmt.Errorf("env must be local, dev or prod")
	}
	if len(c.SigningKey) < 32 {
		return fmt.Errorf("RMS_SIGNING_KEY must contain at least 32 bytes")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("HTTP port must be 1..65535")
	}
	if c.Timeout <= 0 || c.IdleTimeout <= 0 || c.TokenTTL <= 0 || c.RefreshTokenTTL <= 0 {
		return fmt.Errorf("timeouts and TTLs must be positive")
	}
	if c.RefreshTokenTTL <= c.TokenTTL {
		return fmt.Errorf("refresh token TTL must exceed access token TTL")
	}
	if err := c.Scheduler.Validate(); err != nil {
		return err
	}
	return c.Db.Validate()
}
func (c DbSetting) Validate() error {
	if strings.TrimSpace(c.Host) == "" || strings.TrimSpace(c.User) == "" || strings.TrimSpace(c.DbName) == "" {
		return fmt.Errorf("db host, user and dbname are required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("database port must be 1..65535")
	}
	if c.ConnectTimeout <= 0 || c.ConnMaxLifetime <= 0 || c.ConnMaxIdleTime <= 0 {
		return fmt.Errorf("database timeouts must be positive")
	}
	if c.MaxOpenConns < 1 || c.MaxIdleConns < 0 || c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("invalid database pool limits")
	}
	switch c.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("unsupported database sslmode")
	}
	if (c.SSLCert == "") != (c.SSLKey == "") {
		return fmt.Errorf("sslcert and sslkey must be configured together")
	}
	return nil
}
