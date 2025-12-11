package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Cache     CacheConfig     `mapstructure:"cache"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	CORS      CORSConfig      `mapstructure:"cors"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	GeoIP     GeoIPConfig     `mapstructure:"geoip"`
	Webhooks  WebhooksConfig  `mapstructure:"webhooks"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	SAML      SAMLConfig      `mapstructure:"saml"`
	Email     EmailConfig     `mapstructure:"email"`
	Domains   DomainsConfig   `mapstructure:"domains"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type DatabaseConfig struct {
	Global GlobalDBConfig `mapstructure:"global"`
	Tenant TenantDBConfig `mapstructure:"tenant"`
}

type GlobalDBConfig struct {
	URL            string `mapstructure:"url"`
	AuthToken      string `mapstructure:"auth_token"`
	MaxConnections int    `mapstructure:"max_connections"`
}

type TenantDBConfig struct {
	BasePath             string `mapstructure:"base_path"`
	MaxConnectionsPerOrg int    `mapstructure:"max_connections_per_org"`
}

type CacheConfig struct {
	LinkTTL    time.Duration `mapstructure:"link_ttl"`
	MaxEntries int           `mapstructure:"max_entries"`
}

type JWTConfig struct {
	Secret         string        `mapstructure:"secret"`
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	MaxAge         int      `mapstructure:"max_age"`
}

type RateLimitConfig struct {
	RedirectPerMinute int `mapstructure:"redirect_per_minute"`
	APIReadPerMinute  int `mapstructure:"api_read_per_minute"`
	APIWritePerMinute int `mapstructure:"api_write_per_minute"`
}

type GeoIPConfig struct {
	DatabasePath string `mapstructure:"database_path"`
}

type WebhooksConfig struct {
	WorkerCount   int    `mapstructure:"worker_count"`
	RetryAttempts int    `mapstructure:"retry_attempts"`
	RetryBackoff  string `mapstructure:"retry_backoff"`
}

type LoggingConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

type SAMLConfig struct {
	SPEntityID string `mapstructure:"sp_entity_id"`
	SPACSURL   string `mapstructure:"sp_acs_url"`
	SPCertPath string `mapstructure:"sp_cert_path"`
	SPKeyPath  string `mapstructure:"sp_key_path"`
}

type EmailConfig struct {
	Provider string     `mapstructure:"provider"`
	SMTP     SMTPConfig `mapstructure:"smtp"`
}

type SMTPConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	FromAddress string `mapstructure:"from_address"`
	FromName    string `mapstructure:"from_name"`
}

type DomainsConfig struct {
	ShortDomain string `mapstructure:"short_domain"`
	AppDomain   string `mapstructure:"app_domain"`
	APIDomain   string `mapstructure:"api_domain"`
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
