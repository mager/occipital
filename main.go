package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/mager/occipital/handler/health"
	"go.uber.org/fx"
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
		fx.Provide(NewHTTPServer, NewServeMux,
			fx.Annotate(
				health.NewHealthHandler,
				fx.As(new(Route)),
			),
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
func NewServeMux(route Route) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(route.Pattern(), route)
	return mux
}
