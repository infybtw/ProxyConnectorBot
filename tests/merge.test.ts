import { describe, expect, test } from "bun:test";
import { decodeBase64, EmptyBundleError, merge, mergeUserinfo, type Part } from "../src/merge";

const enc = new TextEncoder();
const part = (body: string, headers: Record<string, string> = {}): Part => ({
  body: enc.encode(body),
  header: new Headers(headers),
});
const text = (body: Uint8Array): string => new TextDecoder().decode(body);

describe("merge", () => {
  test("concatenates plain links and drops duplicates, keeping order", () => {
    const res = merge([
      part("vless://a\nvless://b\n"),
      part("  vless://b\r\nvless://c  \n\n"),
    ]);
    expect(text(res.body)).toBe("vless://a\nvless://b\nvless://c");
  });

  test("emits base64 when any part was base64 encoded", () => {
    const first = Buffer.from("trojan://x\nvless://a", "utf8").toString("base64");
    const res = merge([part(first), part("vless://b")]);
    const decoded = Buffer.from(text(res.body), "base64").toString("utf8");
    expect(decoded).toBe("trojan://x\nvless://a\nvless://b");
  });

  test("stays plain when no part was base64 encoded", () => {
    const res = merge([part("vless://a")]);
    expect(text(res.body)).toBe("vless://a");
  });

  test("sums userinfo traffic and keeps the earliest expiry", () => {
    const res = merge([
      part("vless://a", { "Subscription-Userinfo": "upload=10; download=20; total=100; expire=2000", "Profile-Title": "first" }),
      part("vless://b", { "Subscription-Userinfo": "upload=1; download=2; total=50; expire=1000" }),
      part("vless://c", { "Subscription-Userinfo": "upload=5; download=5; total=5" }),
    ]);
    expect(res.header.get("Subscription-Userinfo")).toBe("upload=16; download=27; total=155; expire=1000");
    expect(res.header.get("Profile-Title")).toBe("first");
  });

  test("throws on an empty part list", () => {
    expect(() => merge([])).toThrow(EmptyBundleError);
  });
});

describe("decodeBase64", () => {
  test("decodes standard and url alphabets", () => {
    const std = Buffer.from("vless://a", "utf8").toString("base64");
    expect(decodeBase64(std)).toBe("vless://a");
  });

  test("rejects plain text", () => {
    expect(decodeBase64("vless://a")).toBeNull();
  });

  test("rejects base64 that decodes to non-link text", () => {
    expect(decodeBase64(Buffer.from("hello world", "utf8").toString("base64"))).toBeNull();
  });
});

describe("mergeUserinfo", () => {
  test("ignores malformed fields and non-positive expiry", () => {
    expect(mergeUserinfo(["upload=x; total=10; expire=0"])).toBe("total=10");
  });
});
