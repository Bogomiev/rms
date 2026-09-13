package httpapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	hdlr "rms/internal/app/http/handler"

	mwLogger "rms/internal/http-server/middleware/logger"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type App struct {
	log        *slog.Logger
	hTTPServer *http.Server
	address    string
}

type Config struct {
	TokenTTL        time.Duration
	RefreshTokenTTL time.Duration
	Port            int
	Timeout         time.Duration
	IdleTimeout     time.Duration
}

type Dependencies struct {
	Logger   *slog.Logger
	Auth     hdlr.AuthService
	Users    hdlr.UserService
	Tokens   hdlr.TokenVerifier
	Products hdlr.ProductService
	Stores   hdlr.StoreService
}

func New(cfg Config, deps Dependencies) *App {
	router := chi.NewRouter()
	initMiddlewares(router, deps.Logger)
	handler := hdlr.NewHandler(deps.Auth, deps.Users)
	handler.Configure(deps.Products, deps.Stores, cfg.TokenTTL, cfg.RefreshTokenTTL)
	tokenMaker := deps.Tokens
	router.Get("/usertokenvalid", handler.UserTokenValid)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker)).Get("/products", handler.Products)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker)).Get("/stores", handler.Stores)

	router.Route("/auth", func(r chi.Router) {
		r.Post("/login", handler.Login)
		r.Post("/logout", handler.Logout)
		r.Post("/refresh", handler.RefreshToken)
	})

	router.Route("/users", func(r chi.Router) {
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker)).Get("/", handler.UserList)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker)).Post("/", handler.CreateUser)
	})

	address := fmt.Sprintf("localhost:%d", cfg.Port)

	srv := &http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &App{log: deps.Logger, hTTPServer: srv, address: address}
}

// Run runs HTTP server.
func (a *App) Run() error {
	const op = "httpapp.Run"

	a.log.Info("starting server", slog.String("address", a.address))

	l, err := net.Listen("tcp", a.address)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	a.log.Info("HTTP server started", slog.String("addr", l.Addr().String()))

	if err := a.hTTPServer.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// Stop stops HTTP server.
func (a *App) Stop() error {
	const op = "httpapp.Stop"

	a.log.With(slog.String("op", op)).
		Info("stopping HTTP server", slog.String("port", a.address))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.hTTPServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("%s: %w", op, errors.Join(err, a.hTTPServer.Close()))
	}
	return nil
}

func initMiddlewares(router chi.Router, log *slog.Logger) {
	router.Use(middleware.RequestID)
	router.Use(middleware.ClientIPFromRemoteAddr)
	router.Use(mwLogger.New(log))
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)
}
