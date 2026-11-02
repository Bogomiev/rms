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
	TaskTimezone    string
	AppOrigins      []string
	TokenTTL        time.Duration
	RefreshTokenTTL time.Duration
	Port            int
	Timeout         time.Duration
	IdleTimeout     time.Duration
}

type Dependencies struct {
	Tasks       hdlr.TaskService
	Roles       hdlr.RoleService
	Positions   hdlr.PositionService
	Files       hdlr.FileService
	Discounts   hdlr.DiscountService
	Prices      hdlr.PriceService
	Stocks      hdlr.StockService
	ProductInfo hdlr.ProductInfoService
	Marketplace hdlr.MarketplaceService
	Logger      *slog.Logger
	Auth        hdlr.AuthService
	Users       hdlr.UserService
	Tokens      hdlr.TokenVerifier
	Products    hdlr.ProductService
	Stores      hdlr.StoreService
}

func New(cfg Config, deps Dependencies) *App {
	router := chi.NewRouter()
	initMiddlewares(router, deps.Logger)
	handler := hdlr.NewHandler(deps.Auth, deps.Users)
	handler.Configure(deps.Products, deps.Stores, cfg.TokenTTL, cfg.RefreshTokenTTL)
	handler.ConfigurePriceStock(deps.Prices, deps.Stocks)
	handler.ConfigureDiscounts(deps.Discounts)
	handler.ConfigureProductInfo(deps.ProductInfo, deps.Marketplace)
	handler.ConfigureFiles(deps.Files)
	handler.ConfigureRoles(deps.Roles)
	handler.ConfigurePositions(deps.Positions)
	handler.ConfigureTasks(deps.Tasks)
	handler.ConfigureTaskTimezone(cfg.TaskTimezone)
	tokenMaker := deps.Tokens
	handler.ConfigureSecurity(tokenMaker)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/MarketplaceData", handler.MarketplaceData)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/product_info", handler.ProductInfo)
	router.Get("/usertokenvalid", handler.UserTokenValid)
	router.Get("/version", handler.Version)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/products", handler.Products)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/stores", handler.Stores)

	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/discounts", handler.Discounts)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/prices", handler.Prices)
	router.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/stocks/{store_id}", handler.Stocks)

	router.Route("/files", func(r chi.Router) {
		r.Use(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth))
		r.Get("/", handler.Files)
		r.Get("/by-path", handler.FileByPath)
		r.Get("/{id}", handler.File)
		r.Get("/{id}/content", handler.FileContent)
		r.Post("/", handler.CreateFile)
		r.Put("/{id}", handler.UpdateFile)
		r.Delete("/{id}", handler.DeleteFile)
	})
	router.Route("/auth", func(r chi.Router) {
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/session", handler.Session)
		r.With(hdlr.OriginMiddleware(cfg.AppOrigins...)).Post("/login", handler.Login)
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Post("/logout", handler.Logout)
		r.With(hdlr.OriginMiddleware(cfg.AppOrigins...)).Post("/refresh", handler.RefreshToken)
	})

	router.Route("/users", func(r chi.Router) {
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/", handler.UserList)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Post("/", handler.CreateUser)
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/{id}", handler.UserByID)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Put("/{id}", handler.SaveUser)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Delete("/{id}", handler.DeleteUser)
	})

	router.Route("/roles", func(r chi.Router) {
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/", handler.RoleList)
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/{id}", handler.RoleByID)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Post("/", handler.CreateRole)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Put("/{id}", handler.SaveRole)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Delete("/{id}", handler.DeleteRole)
	})
	router.Route("/positions", func(r chi.Router) {
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/", handler.PositionList)
		r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/{id}", handler.PositionByID)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Post("/", handler.CreatePosition)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Put("/{id}", handler.SavePosition)
		r.With(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth)).Delete("/{id}", handler.DeletePosition)
	})
	for _, kind := range []string{"task_types", "task_scheduling", "tasks", "task_status_history"} {
		router.Route("/"+kind, func(r chi.Router) {
			if kind == "tasks" {
				r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Get("/current", handler.TaskCurrent)
				for _, action := range []string{"start", "cancel", "complete"} {
					r.With(hdlr.GetAuthMiddlewareFunc(tokenMaker, deps.Auth)).Post("/{id}/"+action, handler.TaskAction(action))
				}
			}
			r.Group(func(r chi.Router) {
				r.Use(hdlr.GetAdminMiddlewareFunc(tokenMaker, deps.Auth))
				if kind == "task_types" {
					r.Post("/{id}/generate", handler.TaskGenerateType)
				}
				r.Get("/", handler.TaskResource(kind))
				r.Get("/{id}", handler.TaskResource(kind))
				r.Post("/", handler.TaskResource(kind))
				r.Put("/{id}", handler.TaskResource(kind))
				r.Delete("/{id}", handler.TaskResource(kind))
			})
		})
	}
	address := fmt.Sprintf(":%d", cfg.Port)

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
