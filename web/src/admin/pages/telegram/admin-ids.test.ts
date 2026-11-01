import { describe, expect, it } from "vitest";
import { parseAdminIds } from "./admin-ids";

describe("parseAdminIds", () => {
  it("reads IDs split by commas, spaces and lines", () => {
    expect(parseAdminIds("123, 456\n789  42;7")).toEqual({ ids: [123, 456, 789, 42, 7], bad: [] });
  });

  it("keeps the first of a repeat", () => {
    expect(parseAdminIds("5, 6, 5")).toEqual({ ids: [5, 6], bad: [] });
  });

  it("is empty for an empty text", () => {
    expect(parseAdminIds("  ,, ")).toEqual({ ids: [], bad: [] });
  });

  it("refuses what is not a positive whole ID", () => {
    expect(parseAdminIds("0 -5 1.5 abc 12e3 0x1F 007").bad).toEqual(["0", "-5", "1.5", "abc", "12e3", "0x1F"]);
    expect(parseAdminIds("0 -5 1.5 abc 007").ids).toEqual([7]);
  });

  it("refuses a number too large to be exact", () => {
    expect(parseAdminIds("1234567890123456789").bad).toEqual(["1234567890123456789"]);
  });
});
