package botapp

import "testing"

func TestDetectHWIDMode(t *testing.T) {
	mode, param := detectHWIDMode("https://panel.example.com/sub/abc")
	if mode != "header" || param != "x-hwid" {
		t.Fatalf("plain url: got %s/%s", mode, param)
	}

	mode, param = detectHWIDMode("https://panel.example.com/sub/abc?hwid=OLD")
	if mode != "query" || param != "hwid" {
		t.Fatalf("hwid query: got %s/%s", mode, param)
	}

	mode, param = detectHWIDMode("https://panel.example.com/sub/abc?token=1&HWID=OLD")
	if mode != "query" || param != "HWID" {
		t.Fatalf("uppercase HWID query: got %s/%s", mode, param)
	}
}

func TestValidHWID(t *testing.T) {
	if !validHWID("270DD26E-160D-4257-B8AC-654800E12F24") {
		t.Fatal("uuid must be valid")
	}
	if validHWID("") || validHWID("has space") || validHWID(string(make([]byte, 129))) {
		t.Fatal("invalid values accepted")
	}
}

func TestDefaultName(t *testing.T) {
	if got := defaultName("https://panel.example.com/sub/abc"); got != "panel.example.com" {
		t.Fatalf("defaultName = %q", got)
	}
}
