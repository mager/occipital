package main

import (
	"context"
	"database/sql"
	"net"
	"net/http"

	"github.com/mager/occipital/config"
	"github.com/mager/occipital/database"
	"github.com/mager/occipital/handler/health"
	spotHandler "github.com/mager/occipital/handler/spotify"
	trackHandler "github.com/mager/occipital/handler/track"
	userHandler "github.com/mager/occipital/handler/user"
	"github.com/mager/occipital/musicbrainz"
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

//	@title			Occipital
//	@version		1.0
//	@description	This is the API for occipital

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

// @host		localhost:8080
// @BasePath	/
func main() {
	fx.New(
		fx.Provide(NewHTTPServer,
			config.Options,
			spotify.Options,
			musicbrainz.Options,
			database.Options,

			AsRoute(health.NewHealthHandler),
			AsRoute(userHandler.NewUserHandler),
			AsRoute(spotHandler.NewSearchHandler),
			AsRoute(spotHandler.NewRecommendedTracksHandler),
			AsRoute(trackHandler.NewGetTrackHandler),

			zap.NewProduction,
		),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}

func NewHTTPServer(
	lc fx.Lifecycle,
	logger *zap.Logger,
	db *sql.DB,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
) *http.Server {
	mux := http.NewServeMux()

	jsonHandler := jsonMiddleware(mux)

	srv := &http.Server{Addr: ":8080", Handler: jsonHandler}
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

	// Define handlers
	healthHandler := health.NewHealthHandler(logger, spotifyClient)
	mux.Handle(healthHandler.Pattern(), healthHandler)

	userHandler := userHandler.NewUserHandler(logger, db)
	mux.Handle(userHandler.Pattern(), userHandler)

	spotifySearchHandler := spotHandler.NewSearchHandler(logger, spotifyClient)
	mux.Handle(spotifySearchHandler.Pattern(), spotifySearchHandler)

	spotifyRecommendedTracksHandler := spotHandler.NewRecommendedTracksHandler(logger, spotifyClient)
	mux.Handle(spotifyRecommendedTracksHandler.Pattern(), spotifyRecommendedTracksHandler)

	spotifyGetTrackHandler := trackHandler.NewGetTrackHandler(logger, spotifyClient, musicbrainzClient)
	mux.Handle(spotifyGetTrackHandler.Pattern(), spotifyGetTrackHandler)

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

func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
