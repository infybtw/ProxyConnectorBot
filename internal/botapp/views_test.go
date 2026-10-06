package botapp

import "testing"

// TestKbSubOnlyHasCoreActions locks the user-facing subscription card down to
// test, rename and delete.
func TestKbSubOnlyHasCoreActions(t *testing.T) {
	kb := kbSub(5, "ru")

	got := make([]string, 0)
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				t.Fatalf("button %q has no callback data", btn.Text)
			}
			got = append(got, *btn.CallbackData)
		}
	}

	want := []string{"sub:t:5", "sub:ren:5", "sub:d:5", "subs"}
	if len(got) != len(want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buttons = %v, want %v", got, want)
		}
	}
}
