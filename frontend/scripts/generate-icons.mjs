// Renders the PWA PNG icons from the SVG sources in public/icons using Playwright's Chromium.
// Usage (from frontend/): node scripts/generate-icons.mjs
import { readFile, writeFile } from "node:fs/promises";
import { chromium } from "@playwright/test";

const targets = [
  { source: "icon.svg", out: "icon-192.png", size: 192 },
  { source: "icon.svg", out: "icon-512.png", size: 512 },
  { source: "icon.svg", out: "apple-touch-icon.png", size: 180 },
  { source: "icon-maskable.svg", out: "icon-maskable-512.png", size: 512 },
];

const browser = await chromium.launch();
try {
  for (const { source, out, size } of targets) {
    const svg = await readFile(new URL(`../public/icons/${source}`, import.meta.url), "utf8");
    const page = await browser.newPage({ viewport: { width: size, height: size } });
    await page.setContent(
      `<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
    );
    await writeFile(new URL(`../public/icons/${out}`, import.meta.url), await page.screenshot({ omitBackground: true }));
    await page.close();
  }
} finally {
  await browser.close();
}
