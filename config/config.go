package config

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string

	SpotifyID     string
	SpotifySecret string
}

func ProvideConfig() Config {
	var cfg Config
	err := envconfig.Process("occipital", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}
	return cfg
}

var Options = ProvideConfig
