package web

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// TestMultiOriginSubscription checks that a subscription with several origins
// fetches each one with its own HWID and serves the merged links, and that a
// single remaining origin is passed through unchanged. Requires TEST_DATABASE_URL.
func TestMultiOriginSubscription(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	const hwidA = "AAAA-1111"
	const hwidB = "BBBB-2222"

	originA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-hwid") != hwidA {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=10; expire=5000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte("vless://a\nvless://shared"))))
	}))
	defer originA.Close()

	originB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-hwid") != hwidB {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Subscription-Userinfo", "upload=3; download=4; total=20; expire=4000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("trojan://b\nvless://shared"))
	}))
	defer originB.Close()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const userID int64 = 434343
	if err := st.UpsertUser(ctx, userID, "ru"); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM subscriptions WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE tg_id = $1`, userID)
	})

	sub := &store.Subscription{
		UserID: userID,
		Name:   "multi",
		Origins: []store.Origin{
			{URL: originA.URL, HWID: hwidA, HWIDMode: store.HWIDModeHeader, HWIDParam: "x-hwid"},
			{URL: originB.URL, HWID: hwidB, HWIDMode: store.HWIDModeHeader, HWIDParam: "x-hwid"},
		},
	}
	if err := st.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if len(sub.Origins) != 2 || sub.Origins[0].ID == 0 {
		t.Fatalf("origins not stored: %+v", sub.Origins)
	}

	oc := origin.NewClient(5*time.Second, 1<<20, origin.Device{OS: "android", OSVersion: "14", Model: "Pixel 7", UserAgent: "it"})
	server := NewServer(st, oc)

	body, header := getBody(t, server, "/s/"+sub.Token)
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("merged body is not base64: %q", body)
	}
	if got, want := string(decoded), "vless://a\nvless://shared\ntrojan://b"; got != want {
		t.Fatalf("merged = %q, want %q", got, want)
	}
	if got := header.Get("Subscription-Userinfo"); got != "upload=4; download=6; total=30; expire=4000" {
		t.Fatalf("userinfo = %q", got)
	}
	if got := header.Get("Profile-Title"); got != "multi" {
		t.Fatalf("Profile-Title = %q", got)
	}
	if got := header.Get("X-Hwid"); got != "" {
		t.Fatalf("x-hwid leaked: %q", got)
	}

	devices, err := st.ListDevices(ctx, sub.ID, 10)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices = %+v, err = %v", devices, err)
	}

	// Removing one origin leaves the other one, served as a passthrough.
	if _, err := st.DeleteOrigin(ctx, userID, sub.Origins[0].ID); err != nil {
		t.Fatalf("delete origin: %v", err)
	}
	body, _ = getBody(t, server, "/s/"+sub.Token)
	if body != "trojan://b\nvless://shared" {
		t.Fatalf("single origin body = %q", body)
	}

	// The last origin cannot be removed.
	if _, err := st.DeleteOrigin(ctx, userID, sub.Origins[1].ID); !errors.Is(err, store.ErrLastOrigin) {
		t.Fatalf("delete last origin err = %v, want ErrLastOrigin", err)
	}

	// Adding to a foreign or missing subscription is rejected.
	extra := &store.Origin{URL: originA.URL, HWID: hwidA, HWIDMode: store.HWIDModeHeader, HWIDParam: "x-hwid"}
	if err := st.AddOrigin(ctx, userID, sub.ID+1000, extra); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("add to missing subscription err = %v", err)
	}
	if err := st.AddOrigin(ctx, userID, sub.ID, extra); err != nil {
		t.Fatalf("add origin: %v", err)
	}
	got, err := st.GetSubscription(ctx, userID, sub.ID)
	if err != nil || len(got.Origins) != 2 {
		t.Fatalf("subscription after add = %+v, err = %v", got, err)
	}
}

// getBody performs a GET through the app and returns the body and headers.
func getBody(t *testing.T, server *Server, path string) (string, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp, err := server.App().Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %s = %d, body %q", path, resp.StatusCode, data)
	}
	return string(data), resp.Header
}
