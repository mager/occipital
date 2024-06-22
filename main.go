package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/mager/occipital/config"
	"github.com/mager/occipital/handler/health"
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

			fx.Annotate(
				NewServeMux,
				fx.ParamTags(`group:"routes"`),
			),

			zap.NewProduction,
		),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}

func NewHTTPServer(lc fx.Lifecycle, mux *http.ServeMux) *http.Server {
	srv := &http.Server{Addr: ":8080", Handler: mux}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			fmt.Println("Starting HTTP server at", srv.Addr)
			go srv.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
	return srv
}

// NewServeMux builds a ServeMux that will route requests
// to the given EchoHandler.
func NewServeMux(routes []Route) *http.ServeMux {
	mux := http.NewServeMux()
	for _, route := range routes {
		mux.Handle(route.Pattern(), route)
	}
	return mux
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
