package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	HTTP       HTTPConfig
	DB         DBConfig
	JWT        JWTConfig
	Worker     WorkerConfig
	Conference ConferenceConfig
	Log        LogConfig
}

type HTTPConfig struct {
	Addr            string        `envconfig:"HTTP_ADDR"             default:"0.0.0.0:8080"`
	ReadTimeout     time.Duration `envconfig:"HTTP_READ_TIMEOUT"     default:"5s"`
	WriteTimeout    time.Duration `envconfig:"HTTP_WRITE_TIMEOUT"    default:"10s"`
	IdleTimeout     time.Duration `envconfig:"HTTP_IDLE_TIMEOUT"     default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"HTTP_SHUTDOWN_TIMEOUT" default:"30s"`
}

type DBConfig struct {
	Host           string        `envconfig:"DB_HOST"           required:"true"`
	Port           string        `envconfig:"DB_PORT"           default:"5432"`
	User           string        `envconfig:"DB_USER"           required:"true"`
	Password       string        `envconfig:"DB_PASSWORD"       required:"true"`
	Name           string        `envconfig:"DB_NAME"           required:"true"`
	SSLMode        string        `envconfig:"DB_SSLMODE"        default:"disable"`
	MaxOpenConns   int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns   int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnectTimeout time.Duration `envconfig:"DB_CONNECT_TIMEOUT"        default:"10s"`
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type JWTConfig struct {
	Secret         string        `envconfig:"JWT_SECRET"           required:"true"`
	AccessTokenTTL time.Duration `envconfig:"JWT_ACCESS_TOKEN_TTL" default:"24h"`
	AdminUserID    string        `envconfig:"DUMMY_ADMIN_UUID"     default:"00000000-0000-0000-0000-000000000001"`
	RegularUserID  string        `envconfig:"DUMMY_USER_UUID"      default:"00000000-0000-0000-0000-000000000002"`
}

type WorkerConfig struct {
	SlotGenerationDays int           `envconfig:"WORKER_SLOT_GENERATION_DAYS" default:"7"`
	Interval           time.Duration `envconfig:"WORKER_INTERVAL"             default:"1h"`
}

type ConferenceConfig struct {
	BaseURL    string `envconfig:"CONFERENCE_BASE_URL"    default:"https://conference.mock.local"`
	FailCreate bool   `envconfig:"CONFERENCE_MOCK_FAIL_CREATE" default:"false"`
	FailDelete bool   `envconfig:"CONFERENCE_MOCK_FAIL_DELETE" default:"false"`
}

type LogConfig struct {
	Level string `envconfig:"LOG_LEVEL" default:"debug"`
}

func Load() (*Config, error) {
	var cfg Config

	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return &cfg, nil
}
