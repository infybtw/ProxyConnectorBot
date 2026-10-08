import { describe, expect, test } from "bun:test";
import { kbOriginSettings, kbOrigins, renderOriginSettings, renderOrigins } from "../src/bot/views";
import type { Origin, Subscription } from "../src/store";

const SECRET = "270DD26E-160D-4257-B8AC-654800E12F24";

function origin(over: Partial<Origin> = {}): Origin {
  return {
    id: 7,
    subscriptionId: 3,
    url: "https://panel.example.com/sub/abc",
    hwid: SECRET,
    hwidMode: "header",
    hwidParam: "x-hwid",
    enabled: true,
    createdAt: new Date(2026, 0, 2, 15, 4),
    ...over,
  };
}

function subscription(origins: Origin[]): Subscription {
  return { id: 3, userId: 1, name: "test", token: "tok", createdAt: new Date(), origins };
}

/** Reads callback_data from a keyboard button (union includes GameButton). */
function dataOf(button: unknown): string | undefined {
  return (button as { callback_data?: string } | undefined)?.callback_data;
}

describe("origin views never expose the HWID", () => {
  test("renderOrigins hides the HWID", () => {
    const text = renderOrigins([origin()], "ru");
    expect(text).not.toContain(SECRET);
    expect(text).toContain("panel.example.com");
  });

  test("renderOriginSettings hides the HWID and shows the status", () => {
    const text = renderOriginSettings(origin(), "ru");
    expect(text).not.toContain(SECRET);
    expect(text).toContain("включён");
  });

  test("renderOriginSettings marks a disabled origin", () => {
    expect(renderOriginSettings(origin({ enabled: false }), "ru")).toContain("выключен");
  });
});

describe("origin keyboards", () => {
  test("each origin opens its settings screen", () => {
    const keyboard = kbOrigins(subscription([origin()]), "ru");
    expect(dataOf(keyboard.inline_keyboard[0]![0])).toBe("sub:or:7");
  });

  test("the settings screen toggles and deletes by origin id", () => {
    const enabled = kbOriginSettings(origin(), 3, "ru");
    expect(dataOf(enabled.inline_keyboard[0]![0])).toBe("sub:oe:7");
    expect(dataOf(enabled.inline_keyboard[1]![0])).toBe("sub:og:7");
    expect(dataOf(enabled.inline_keyboard[2]![0])).toBe("sub:o:3");
  });
});
