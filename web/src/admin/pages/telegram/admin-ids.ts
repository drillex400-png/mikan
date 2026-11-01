// The bot admins' Telegram IDs, as the admin types them.

/** Telegram IDs in a text the admin typed: separated by spaces, commas or lines. */
export function parseAdminIds(text: string): { ids: number[]; bad: string[] } {
  const ids: number[] = [];
  const bad: string[] = [];
  for (const part of text.split(/[\s,;]+/).filter(Boolean)) {
    const n = Number(part);
    if (/^\d{1,15}$/.test(part) && Number.isSafeInteger(n) && n > 0) {
      if (!ids.includes(n)) ids.push(n);
    } else {
      bad.push(part);
    }
  }
  return { ids, bad };
}
