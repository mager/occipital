package main

import (
	"context"
	"net"
	"net/http"

	"github.com/mager/occipital/config"
	"github.com/mager/occipital/handler/health"
	spotHandler "github.com/mager/occipital/handler/spotify"
	"github.com/mager/occipital/spotify"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Route is an http.Handler that knows the mux pattern
// under which it will be registered.
type Route interface {
	http.Handler

	// Pattern reports the path at which this is registered.
	Pattern() string
}

func main() {
	fx.New(
		fx.Provide(NewHTTPServer,
			config.Options,
			spotify.Options,

			AsRoute(health.NewHealthHandler),

			zap.NewProduction,
		),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}

func NewHTTPServer(lc fx.Lifecycle, spotifyClient *spotify.SpotifyClient, logger *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":8080", Handler: mux}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			logger.Sugar().Infof("Starting HTTP server at", srv.Addr)
			go srv.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})

	healthHandler := health.NewHealthHandler(logger, spotifyClient)
	mux.Handle(healthHandler.Pattern(), healthHandler)

	spotifySearchHandler := spotHandler.NewSearchHandler(logger, spotifyClient)
	mux.Handle(spotifySearchHandler.Pattern(), spotifySearchHandler)

	return srv
}

// AsRoute annotates the given constructor to state that
// it provides a route to the "routes" group.
func AsRoute(f any) any {
	return fx.Annotate(
		f,
		fx.As(new(Route)),
		fx.ResultTags(`group:"routes"`),
	)
}
