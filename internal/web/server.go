// Package web serves subscriptions over HTTP on our own domain.
package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"

	"github.com/infybtw/ProxyConnectorBot/internal/merge"
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

// fetchResult is the outcome of fetching one origin.
type fetchResult struct {
	res *origin.Result
	err error
}

// handleSubscription serves GET /s/:token. A subscription with a single origin
// is passed through as-is. With several origins each one is fetched with its
// own HWID and the share links are merged; origins that fail are skipped as
// long as one of them answers.
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
	if len(sub.Origins) == 0 {
		return ctx.Status(fiber.StatusNotFound).SendString("subscription has no origins")
	}

	// Best effort: remember which device fetched the subscription, even if
	// the origin requests below fail.
	if err := s.store.TouchDevice(ctx.Context(), sub.ID, deviceInfo(ctx)); err != nil {
		slog.Warn("web: record device failed", "sub_id", sub.ID, "err", err)
	}

	results := s.fetchOrigins(ctx.Context(), sub)

	if len(sub.Origins) == 1 {
		r := results[0]
		if r.err != nil {
			return ctx.Status(fiber.StatusBadGateway).SendString("origin unavailable")
		}
		slog.Info("web: subscription served",
			"sub_id", sub.ID, "status", r.res.StatusCode, "size", len(r.res.Body))
		copyHeaders(ctx, r.res.Header)
		return ctx.Status(r.res.StatusCode).Send(r.res.Body)
	}

	parts := make([]merge.Part, 0, len(results))
	for _, r := range results {
		switch {
		case r.err != nil:
			continue
		case r.res.StatusCode != fiber.StatusOK:
			slog.Warn("web: origin returned non-200", "sub_id", sub.ID, "status", r.res.StatusCode)
			continue
		}
		parts = append(parts, merge.Part{Body: r.res.Body, Header: r.res.Header})
	}
	if len(parts) == 0 {
		return ctx.Status(fiber.StatusBadGateway).SendString("origin unavailable")
	}
	merged, err := merge.Merge(parts)
	if err != nil {
		slog.Error("web: merge failed", "sub_id", sub.ID, "err", err)
		return ctx.Status(fiber.StatusBadGateway).SendString("origin unavailable")
	}

	slog.Info("web: subscription served",
		"sub_id", sub.ID, "origins", len(sub.Origins), "merged", len(parts), "size", len(merged.Body))

	copyHeaders(ctx, merged.Header)
	ctx.Set("Profile-Title", sub.Name)
	return ctx.Status(fiber.StatusOK).Send(merged.Body)
}

// fetchOrigins requests every origin of the subscription concurrently. The
// result slice keeps the order of sub.Origins.
func (s *Server) fetchOrigins(ctx context.Context, sub store.Subscription) []fetchResult {
	out := make([]fetchResult, len(sub.Origins))
	var wg sync.WaitGroup
	for i, o := range sub.Origins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := s.origin.Fetch(ctx, o)
			if err != nil {
				slog.Error("web: origin fetch failed", "sub_id", sub.ID, "origin_id", o.ID, "err", err)
			}
			out[i] = fetchResult{res: res, err: err}
		}()
	}
	wg.Wait()
	return out
}

// copyHeaders passes origin headers to the client, minus hop-by-hop and
// identity headers.
func copyHeaders(ctx fiber.Ctx, header http.Header) {
	for key, values := range header {
		lower := strings.ToLower(key)
		if hopByHopHeaders[lower] {
			continue
		}
		for _, v := range values {
			ctx.Set(key, v)
		}
	}
}

// Metadata limits protect the database from oversized or abusive headers.
const (
	maxHWIDLen  = 128
	maxUALen    = 512
	maxOSLen    = 64
	maxModelLen = 128
	maxIPLen    = 64
)

// deviceInfo extracts the identity of the client that made the request.
func deviceInfo(ctx fiber.Ctx) store.DeviceInfo {
	hwid := truncate(firstNonEmpty(ctx.Get("x-hwid"), ctx.Query("hwid")), maxHWIDLen)
	ua := truncate(ctx.Get("User-Agent"), maxUALen)
	os := truncate(ctx.Get("x-device-os"), maxOSLen)
	osVer := truncate(ctx.Get("x-ver-os"), maxOSLen)
	model := truncate(ctx.Get("x-device-model"), maxModelLen)
	ip := truncate(clientIP(ctx), maxIPLen)

	key := "hwid:" + hwid
	if hwid == "" {
		key = "ua:" + fingerprint(ua, os, osVer, model)
	}
	return store.DeviceInfo{
		Key:       key,
		HWID:      hwid,
		UserAgent: ua,
		OS:        os,
		OSVersion: osVer,
		Model:     model,
		IP:        ip,
	}
}

// clientIP prefers the X-Forwarded-For address set by the reverse proxy.
func clientIP(ctx fiber.Ctx) string {
	if xff := ctx.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	return ctx.IP()
}

// fingerprint builds a short stable id from device metadata for clients that
// do not send a HWID.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// truncate caps s to n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
