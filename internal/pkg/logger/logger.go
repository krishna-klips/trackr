package logger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"trackr/internal/platform/config"
)

func Init(cfg config.LoggingConfig) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	var level zerolog.Level
	switch cfg.Level {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	default:
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.Output == "file" && cfg.FilePath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0755); err != nil {
			log.Error().Err(err).Msg("failed to create log directory")
			// fallback to stdout
			return
		}

		file, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			log.Error().Err(err).Msg("failed to open log file")
			return
		}
		log.Logger = zerolog.New(file).With().Timestamp().Logger()
	} else if cfg.Format == "text" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	} else {
		// JSON format to stdout (default)
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
}
