package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/mager/occipital/config"
	"github.com/mager/occipital/database"
	fs "github.com/mager/occipital/firestore"
	creatorHandler "github.com/mager/occipital/handler/creator"
	discoverHandler "github.com/mager/occipital/handler/discover"
	"github.com/mager/occipital/handler/genre"
	"github.com/mager/occipital/handler/health"
	podcastHandler "github.com/mager/occipital/handler/podcast"
	profileHandler "github.com/mager/occipital/handler/profile"
	spotHandler "github.com/mager/occipital/handler/spotify"
	trackHandler "github.com/mager/occipital/handler/track"
	userHandler "github.com/mager/occipital/handler/user"
	"github.com/mager/occipital/logger"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/spotify"
	"github.com/mager/occipital/util"
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
			database.Options,
			fs.Options,
			spotify.Options,
			musicbrainz.Options,
			logger.Options,

			AsRoute(health.NewHealthHandler),
			AsRoute(userHandler.NewUserHandler),
			AsRoute(profileHandler.NewProfileHandler),
			AsRoute(spotHandler.NewSearchHandler),
			AsRoute(spotHandler.NewRecommendedTracksHandler),
			AsRoute(trackHandler.NewGetTrackHandler),
			AsRoute(discoverHandler.NewDiscoverV2Handler),
			AsRoute(genre.NewGenreHandler),
			AsRoute(podcastHandler.NewCategoriesHandler),
			AsRoute(podcastHandler.NewShowsHandler),
			AsRoute(creatorHandler.NewGetCreatorHandler),
		),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}

func NewHTTPServer(
	lc fx.Lifecycle,
	cfg config.Config,
	db *sql.DB,
	fs *firestore.Client,
	spotifyClient *spotify.SpotifyClient,
	musicbrainzClient *musicbrainz.MusicbrainzClient,
	logger *zap.SugaredLogger,
) *http.Server {
	router := mux.NewRouter()

	router.Use(jsonMiddleware)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr: ":" + port, Handler: apiKeyMiddleware(router, os.Getenv("BEATBRAIN_API_KEY")),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			logger.Infof("Starting HTTP server at %s", srv.Addr)
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

	spotifyGetTrackHandler := trackHandler.NewGetTrackHandler(logger, spotifyClient, musicbrainzClient)
	router.Handle(spotifyGetTrackHandler.Pattern(), spotifyGetTrackHandler)

	// v2: parallel + cached track handler
	trackV2Handler := trackHandler.NewGetTrackV2Handler(logger, spotifyClient, musicbrainzClient, fs)
	router.Handle(trackV2Handler.Pattern(), trackV2Handler)

	discoverV2Handler := discoverHandler.NewDiscoverV2Handler(logger, fs)
	router.Handle(discoverV2Handler.Pattern(), discoverV2Handler)

	genreHandler := genre.NewGenreHandler(logger, spotifyClient, musicbrainzClient)
	router.Handle(genreHandler.Pattern(), genreHandler)

	getCreatorHandler := creatorHandler.NewGetCreatorHandler(logger, musicbrainzClient, spotifyClient)
	router.Handle(getCreatorHandler.Pattern(), getCreatorHandler)

	// Podcast handlers
	podcastCategoriesHandler := podcastHandler.NewCategoriesHandler(logger, fs)
	router.Handle(podcastCategoriesHandler.Pattern(), podcastCategoriesHandler)

	podcastShowsHandler := podcastHandler.NewShowsHandler(logger, fs)
	router.Handle(podcastShowsHandler.Pattern(), podcastShowsHandler)

	// Spotify OAuth handlers
	authLoginHandler := spotHandler.NewAuthLoginHandler(logger, cfg)
	router.Handle(authLoginHandler.Pattern(), authLoginHandler)

	authCallbackHandler := spotHandler.NewAuthCallbackHandler(logger, cfg, fs)
	router.Handle(authCallbackHandler.Pattern(), authCallbackHandler)

	// Spotify playback handlers
	playHandler := spotHandler.NewPlayHandler(logger, cfg, fs)
	router.Handle(playHandler.Pattern(), playHandler)

	pauseHandler := spotHandler.NewPauseHandler(logger, cfg, fs)
	router.Handle(pauseHandler.Pattern(), pauseHandler)

	devicesHandler := spotHandler.NewDevicesHandler(logger, cfg, fs)
	router.Handle(devicesHandler.Pattern(), devicesHandler)

	// websocket handler
	router.HandleFunc("/np", func(w http.ResponseWriter, r *http.Request) {
		spotHandler.PlayerHandler(w, r, spotifyClient, logger)
	})

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

// apiKeyMiddleware permits data access only through the trusted app server.
// A missing configuration fails closed in local and deployed environments.
func apiKeyMiddleware(next http.Handler, key string) http.Handler {
	expected := sha256.Sum256([]byte(key))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		provided := sha256.Sum256([]byte(r.Header.Get("X-Beatbrain-Key")))
		if key == "" || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			util.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

var (
	nextSecret = os.Getenv("OCCIPITAL_NEXTAUTHSECRET")
)

// authNMiddleware authenticates the request
func authNMiddleware(next http.Handler, logger *zap.SugaredLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := nextSecret
		tokenString := extractTokenFromHeader(r.Header.Get("Authorization"))
		if secret == "" || tokenString == "" {
			util.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil {
			util.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			logger.Infow("Successful authentication", "email", claims["email"])
		} else {
			util.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Extracts the token value from the Authorization header
func extractTokenFromHeader(header string) string {
	// Split the header value by whitespace
	split := strings.Fields(header)

	if len(split) != 2 || strings.ToLower(split[0]) != "bearer" {
		return ""
	}

	return split[1]
}
