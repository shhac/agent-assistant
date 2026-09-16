import { describe, expect, it } from "vitest";
import { dateLabel, fullDateLabel, recordedTime, sinceLabel } from "./ui";

describe("recorded times", () => {
  // Go serializes an unset time as year one, so it arrives looking like a real
  // timestamp. Treating it as one prints "Jan 1, 12:00 AM" for "never".
  it("treats the daemon's unset time as no time at all", () => {
    expect(recordedTime("0001-01-01T00:00:00Z")).toBeNull();
    expect(dateLabel("0001-01-01T00:00:00Z")).toBe("");
    expect(fullDateLabel("0001-01-01T00:00:00Z")).toBe("");
    expect(sinceLabel("0001-01-01T00:00:00Z")).toBe("");
  });

  it("returns nothing for missing or unparseable input", () => {
    for (const value of [undefined, "", "not a date"]) {
      expect(recordedTime(value)).toBeNull();
      expect(dateLabel(value)).toBe("");
      expect(sinceLabel(value)).toBe("");
    }
  });

  it("describes elapsed time in the owner's terms", () => {
    const ago = (minutes: number) =>
      new Date(Date.now() - minutes * 60_000).toISOString();
    expect(sinceLabel(ago(0))).toBe("just now");
    expect(sinceLabel(ago(45))).toBe("45 min ago");
    expect(sinceLabel(ago(60))).toBe("1 hour ago");
    expect(sinceLabel(ago(60 * 48))).toBe("2 days ago");
  });

  // Recorded times come from several clocks, so a slightly future stamp is an
  // expected input rather than a hypothetical.
  it("does not present a future timestamp as elapsed time", () => {
    for (const skew of [30_000, 5 * 60_000, 60 * 60_000]) {
      const ahead = new Date(Date.now() + skew).toISOString();
      expect(sinceLabel(ahead)).toBe("just now");
      expect(dateLabel(ahead)).not.toBe("");
    }
  });
});
