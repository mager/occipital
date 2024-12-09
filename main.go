package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/mager/occipital/config"
	"github.com/mager/occipital/database"
	fs "github.com/mager/occipital/firestore"
	discoverHandler "github.com/mager/occipital/handler/discover"
	"github.com/mager/occipital/handler/health"
	profileHandler "github.com/mager/occipital/handler/profile"
	spotHandler "github.com/mager/occipital/handler/spotify"
	trackHandler "github.com/mager/occipital/handler/track"
	userHandler "github.com/mager/occipital/handler/user"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/musixmatch"
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
			fs.Options,
			musixmatch.Options,
			database.Options,

			AsRoute(health.NewHealthHandler),
			AsRoute(userHandler.NewUserHandler),
			AsRoute(profileHandler.NewProfileHandler),
			AsRoute(spotHandler.NewSearchHandler),
			AsRoute(spotHandler.NewRecommendedTracksHandler),
			AsRoute(trackHandler.NewGetTrackHandler),
			AsRoute(discoverHandler.NewDiscoverHandler),

			zap.NewProduction,
		),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}

func NewHTTPServer(
	lc fx.Lifecycle,
	logger *zap.Logger,
	db *sql.DB,
	fs *firestore.Client,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
	musixmatchClient *musixmatch.MusixmatchClient,
) *http.Server {
	router := mux.NewRouter()

	router.Use(jsonMiddleware)
	srv := &http.Server{Addr: ":8080", Handler: router}
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
	router.Handle(healthHandler.Pattern(), healthHandler)

	userHandler := userHandler.NewUserHandler(logger, db)
	router.Handle(userHandler.Pattern(), authNMiddleware(userHandler, logger))

	profileHandler := profileHandler.NewProfileHandler(logger, db)
	router.Handle(profileHandler.Pattern(), profileHandler)

	spotifySearchHandler := spotHandler.NewSearchHandler(logger, spotifyClient)
	router.Handle(spotifySearchHandler.Pattern(), spotifySearchHandler)

	spotifyRecommendedTracksHandler := spotHandler.NewRecommendedTracksHandler(logger, spotifyClient)
	router.Handle(spotifyRecommendedTracksHandler.Pattern(), spotifyRecommendedTracksHandler)

	spotifyGetTrackHandler := trackHandler.NewGetTrackHandler(logger, spotifyClient, musicbrainzClient, musixmatchClient)
	router.Handle(spotifyGetTrackHandler.Pattern(), spotifyGetTrackHandler)

	discoverHandler := discoverHandler.NewDiscoverHandler(logger, fs, spotifyClient)
	router.Handle(discoverHandler.Pattern(), discoverHandler)

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

var (
	nextSecret = os.Getenv("OCCIPITAL_NEXTAUTHSECRET")
)

// authNMiddleware authenticates the request
func authNMiddleware(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := nextSecret
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			logger.Info("Error: No token")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		tokenString = extractTokenFromHeader(tokenString)

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			logger.Sugar().Infow("Successful authentication", "email", claims["email"])
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Extracts the token value from the Authorization header
func extractTokenFromHeader(header string) string {
	// Split the header value by whitespace
	split := strings.SplitN(header, " ", 2)

	if len(split) != 2 || strings.ToLower(split[0]) != "bearer" {
		log.Fatal("Invalid Authorization header format")
	}

	return split[1]
}
