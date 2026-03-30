package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func New(level, service, environment string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	parsedLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		parsedLevel = zerolog.InfoLevel
	}

	return zerolog.New(os.Stdout).
		Level(parsedLevel).
		With().
		Timestamp().
		Str("service", service).
		Str("environment", environment).
		Logger()
}
