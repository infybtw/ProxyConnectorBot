import { describe, expect, test } from "bun:test";
import { HWID_MODE_HEADER, HWID_MODE_QUERY } from "../src/store";
import { defaultName, detectHwidMode, isHttpUrl } from "../src/url";

describe("detectHwidMode", () => {
  test("defaults to the x-hwid header for a plain URL", () => {
    expect(detectHwidMode("https://panel.example.com/sub/abc")).toEqual({
      mode: HWID_MODE_HEADER,
      param: "x-hwid",
    });
  });

  test("mirrors a lowercase hwid query parameter", () => {
    expect(detectHwidMode("https://panel.example.com/sub/abc?hwid=OLD")).toEqual({
      mode: HWID_MODE_QUERY,
      param: "hwid",
    });
  });

  test("mirrors an uppercase HWID query parameter", () => {
    expect(detectHwidMode("https://panel.example.com/sub/abc?token=1&HWID=OLD")).toEqual({
      mode: HWID_MODE_QUERY,
      param: "HWID",
    });
  });
});

describe("defaultName", () => {
  test("uses the URL host", () => {
    expect(defaultName("https://panel.example.com/sub/abc")).toBe("panel.example.com");
  });

  test("falls back for an invalid URL", () => {
    expect(defaultName("not a url")).toBe("subscription");
  });
});

describe("isHttpUrl", () => {
  test("accepts http and https", () => {
    expect(isHttpUrl("http://example.com")).toBe(true);
    expect(isHttpUrl("https://example.com/sub")).toBe(true);
  });

  test("rejects other schemes and garbage", () => {
    expect(isHttpUrl("ftp://example.com")).toBe(false);
    expect(isHttpUrl("example.com")).toBe(false);
    expect(isHttpUrl("not a url")).toBe(false);
  });
});
