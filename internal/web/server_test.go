package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// TestSubscriptionPassthrough checks the full HTTP path: lookup by token,
// origin fetch with the stored HWID and transparent response pass-through.
// Requires TEST_DATABASE_URL pointing at a disposable Postgres.
func TestSubscriptionPassthrough(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	const wantHWID = "270DD26E-160D-4257-B8AC-654800E12F24"

	originSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-hwid"); got != wantHWID {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("missing hwid"))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Profile-Title", "Test Sub")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://example"))
	}))
	defer originSrv.Close()

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

	const userID int64 = 424242
	if err := st.UpsertUser(ctx, userID, "ru"); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	sub := &store.Subscription{
		UserID:    userID,
		Name:      "it-test",
		OriginURL: originSrv.URL,
		HWID:      wantHWID,
		HWIDMode:  store.HWIDModeHeader,
		HWIDParam: "x-hwid",
	}
	if err := st.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM subscriptions WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE tg_id = $1`, userID)
	})

	oc := origin.NewClient(5*time.Second, 1<<20, origin.Device{
		OS: "android", OSVersion: "14", Model: "Pixel 7", UserAgent: "it",
	})
	server := NewServer(st, oc)

	req := httptest.NewRequest(http.MethodGet, "/s/"+sub.Token, nil)
	req.Header.Set("User-Agent", "v2rayNG/1.8.5")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	resp, err := server.App().Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %q", resp.StatusCode, body)
	}
	if string(body) != "vless://example" {
		t.Fatalf("body = %q", body)
	}
	if got := resp.Header.Get("Profile-Title"); got != "Test Sub" {
		t.Fatalf("Profile-Title = %q", got)
	}
	if got := resp.Header.Get("X-Hwid"); got != "" {
		t.Fatalf("x-hwid leaked to client: %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/s/does-not-exist", nil)
	resp, err = server.App().Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token status = %d", resp.StatusCode)
	}

	// Device tracking: the client without HWID is recorded by metadata.
	devices, err := st.ListDevices(ctx, sub.ID, 10)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	if devices[0].HWID != "" {
		t.Fatalf("anonymous device recorded with hwid %q", devices[0].HWID)
	}
	if devices[0].UserAgent != "v2rayNG/1.8.5" || devices[0].IP != "203.0.113.7" || devices[0].Requests != 1 {
		t.Fatalf("anonymous device = %+v", devices[0])
	}

	// A client that sends a HWID is tracked separately, and repeated requests
	// bump the counter instead of creating a new device.
	for i := 0; i < 2; i++ {
		req = httptest.NewRequest(http.MethodGet, "/s/"+sub.Token, nil)
		req.Header.Set("x-hwid", wantHWID)
		req.Header.Set("User-Agent", "Happ/2.4.1")
		resp, err = server.App().Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("hwid client status = %d", resp.StatusCode)
		}
	}

	devices, err = st.ListDevices(ctx, sub.ID, 10)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(devices))
	}
	var hwidDev *store.Device
	for i := range devices {
		if devices[i].HWID == wantHWID {
			hwidDev = &devices[i]
		}
	}
	if hwidDev == nil {
		t.Fatal("device with hwid not found")
	}
	if hwidDev.Requests != 2 || hwidDev.UserAgent != "Happ/2.4.1" {
		t.Fatalf("hwid device = %+v", *hwidDev)
	}

	if strings.TrimSpace(sub.Token) == "" {
		t.Fatal("empty token")
	}
}
