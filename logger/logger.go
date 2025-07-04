package logger

import (
	"encoding/json"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// ProvideLogger provides a zap logger
func ProvideLogger() *zap.SugaredLogger {
	rawJSON := []byte(`{
	  "level": "debug",
	  "encoding": "json",
	  "outputPaths": ["stdout", "/tmp/logs"],
	  "errorOutputPaths": ["stderr"],
	  "encoderConfig": {
	    "messageKey": "message",
	    "levelKey": "level",
	    "levelEncoder": "lowercase"
	  }
	}`)

	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	logger := zap.Must(cfg.Build())
	defer logger.Sync()

	return logger.Sugar()
}

// NewTestLogger returns a new logger and observed logs for testing.
func NewTestLogger() (*zap.SugaredLogger, *observer.ObservedLogs) {
	core, recorded := observer.New(zap.InfoLevel)
	return zap.New(core).Sugar(), recorded
}

var Options = ProvideLogger
