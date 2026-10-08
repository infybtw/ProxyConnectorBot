package merge

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func TestMergePlainAndDedupe(t *testing.T) {
	res, err := Merge([]Part{
		{Body: []byte("vless://a\nvless://b\n"), Header: http.Header{}},
		{Body: []byte("  vless://b\r\nvless://c  \n\n"), Header: http.Header{}},
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got, want := string(res.Body), "vless://a\nvless://b\nvless://c"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestMergeBase64Output(t *testing.T) {
	first := base64.StdEncoding.EncodeToString([]byte("trojan://x\nvless://a"))
	res, err := Merge([]Part{
		{Body: []byte(first), Header: http.Header{}},
		{Body: []byte("vless://b"), Header: http.Header{}},
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(res.Body))
	if err != nil {
		t.Fatalf("output is not base64: %v", err)
	}
	if got, want := string(decoded), "trojan://x\nvless://a\nvless://b"; got != want {
		t.Fatalf("decoded = %q, want %q", got, want)
	}
}

func TestMergePlainStaysPlainWhenNoBase64(t *testing.T) {
	res, err := Merge([]Part{{Body: []byte("vless://a"), Header: http.Header{}}})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if string(res.Body) != "vless://a" {
		t.Fatalf("body = %q", res.Body)
	}
}

func TestMergeUserinfo(t *testing.T) {
	h1 := http.Header{}
	h1.Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=2000")
	h1.Set("Profile-Title", "first")
	h2 := http.Header{}
	h2.Set("Subscription-Userinfo", "upload=1; download=2; total=50; expire=1000")
	h3 := http.Header{}
	h3.Set("Subscription-Userinfo", "upload=5; download=5; total=5")

	res, err := Merge([]Part{
		{Body: []byte("vless://a"), Header: h1},
		{Body: []byte("vless://b"), Header: h2},
		{Body: []byte("vless://c"), Header: h3},
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	want := "upload=16; download=27; total=155; expire=1000"
	if got := res.Header.Get("Subscription-Userinfo"); got != want {
		t.Fatalf("userinfo = %q, want %q", got, want)
	}
	if got := res.Header.Get("Profile-Title"); got != "first" {
		t.Fatalf("Profile-Title = %q", got)
	}
}

func TestMergeEmpty(t *testing.T) {
	if _, err := Merge(nil); err != ErrEmpty {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
}
