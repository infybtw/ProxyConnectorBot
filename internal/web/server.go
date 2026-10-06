// Package web serves subscriptions over HTTP on our own domain.
package web

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// hopByHopHeaders must not be forwarded, see RFC 7230 section 6.1.
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
	// Set by us or computed by fiber:
	"content-length": true,
	"set-cookie":     true,
	// Identity headers we never leak to clients:
	"x-hwid":         true,
	"x-device-os":    true,
	"x-ver-os":       true,
	"x-device-model": true,
}

// Server is the HTTP surface of the application.
type Server struct {
	app    *fiber.App
	store  *store.Store
	origin *origin.Client
}

// NewServer wires routes on a fresh fiber app.
func NewServer(st *store.Store, oc *origin.Client) *Server {
	s := &Server{
		app: fiber.New(fiber.Config{
			AppName: "ProxyConnectorBot",
		}),
		store:  st,
		origin: oc,
	}

	s.app.Get("/healthz", s.handleHealth)
	s.app.Get("/s/:token", s.handleSubscription)
	return s
}

// App exposes the underlying fiber app (for graceful shutdown).
func (s *Server) App() *fiber.App { return s.app }

// Listen starts the HTTP server (blocking).
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

// Shutdown stops the HTTP server gracefully.
func (s *Server) Shutdown() error {
	return s.app.ShutdownWithContext(context.Background())
}

func (s *Server) handleHealth(ctx fiber.Ctx) error {
	if err := s.store.Ping(ctx.Context()); err != nil {
		return ctx.Status(fiber.StatusServiceUnavailable).SendString("db unavailable")
	}
	return ctx.SendString("ok")
}

// handleSubscription proxies GET /s/:token to the origin subscription with
// the stored HWID attached, passing the response through as-is.
func (s *Server) handleSubscription(ctx fiber.Ctx) error {
	token := ctx.Params("token")
	sub, err := s.store.GetByToken(ctx.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ctx.Status(fiber.StatusNotFound).SendString("subscription not found")
		}
		slog.Error("web: lookup failed", "token", token, "err", err)
		return ctx.Status(fiber.StatusInternalServerError).SendString("internal error")
	}

	res, err := s.origin.Fetch(ctx.Context(), sub)
	if err != nil {
		slog.Error("web: origin fetch failed", "sub_id", sub.ID, "err", err)
		return ctx.Status(fiber.StatusBadGateway).SendString("origin unavailable")
	}

	slog.Info("web: subscription served",
		"sub_id", sub.ID, "status", res.StatusCode, "size", len(res.Body))

	for key, values := range res.Header {
		lower := strings.ToLower(key)
		if hopByHopHeaders[lower] {
			continue
		}
		for _, v := range values {
			ctx.Set(key, v)
		}
	}
	return ctx.Status(res.StatusCode).Send(res.Body)
}
