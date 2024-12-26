package musixmatch

import (
	"net/http"

	mxm "github.com/mager/go-musixmatch"
	"github.com/mager/occipital/config"
	"go.uber.org/zap"
)

type MusixmatchClient struct {
	Client *mxm.Client
}

func ProvideMusixmatch(cfg config.Config, l *zap.SugaredLogger) *MusixmatchClient {
	var c MusixmatchClient
	c.Client = mxm.New(cfg.MusixmatchAPIKey, http.DefaultClient)
	return &c
}

var Options = ProvideMusixmatch
