#!/usr/bin/env node
/**
 * Generates the SiegaNet app icon (icon-source.png, 1024x1024) with zero
 * native dependencies: a purple power glyph with a soft glow on the theme's
 * near-black rounded square. Regenerate the full icon set afterwards with:
 *
 *   node scripts/gen-icon.mjs && npx tauri icon icon-source.png
 */
import { deflateSync } from "node:zlib";
import { writeFileSync } from "node:fs";

const S = 1024;
const px = new Uint8Array(S * S * 4);

// theme tokens (design/tokens.json)
const BG = [0x0b, 0x09, 0x10];
const ACCENT = [0x8b, 0x5c, 0xf6];
const LIGHT = [0xa7, 0x8b, 0xfa];

const cx = S / 2;
const cy = S / 2;
const cornerR = S * 0.22;

// power glyph geometry
const ringR = S * 0.24;
const ringW = S * 0.062;
const gapDeg = 55; // half-angle of the top gap
const barH = S * 0.30;
const barW = ringW;

function roundedAlpha(x, y) {
  // alpha of the rounded-square background at (x, y)
  const dx = Math.max(Math.abs(x - cx) - (S / 2 - cornerR), 0);
  const dy = Math.max(Math.abs(y - cy) - (S / 2 - cornerR), 0);
  const d = Math.hypot(dx, dy) - cornerR;
  return Math.max(0, Math.min(1, 0.5 - d)); // 1px anti-alias
}

function glyphAlpha(x, y) {
  const dx = x - cx;
  const dy = y - cy;
  const r = Math.hypot(dx, dy);

  // ring with a gap at the top
  const ringD = Math.abs(r - ringR) - ringW / 2;
  let a = 0;
  if (ringD < 1) {
    const angle = (Math.atan2(dx, -dy) * 180) / Math.PI; // 0 = straight up
    if (Math.abs(angle) > gapDeg) a = Math.max(a, Math.min(1, 0.5 - ringD));
  }

  // vertical bar from the centre upwards, rounded caps
  const byTop = cy - barH;
  const clampedY = Math.min(Math.max(y, byTop), cy - S * 0.02);
  const barD = Math.hypot(x - cx, y - clampedY) - barW / 2;
  a = Math.max(a, Math.min(1, 0.5 - barD));
  return Math.max(0, a);
}

for (let y = 0; y < S; y++) {
  for (let x = 0; x < S; x++) {
    const i = (y * S + x) * 4;
    const bgA = roundedAlpha(x, y);
    if (bgA <= 0) continue; // fully transparent corner

    const r = Math.hypot(x - cx, y - cy);
    // radial glow behind the glyph
    const glow = Math.exp(-((r / (S * 0.34)) ** 2)) * 0.55;
    let cr = BG[0] + (ACCENT[0] - BG[0]) * glow;
    let cg = BG[1] + (ACCENT[1] - BG[1]) * glow;
    let cb = BG[2] + (ACCENT[2] - BG[2]) * glow;

    const g = glyphAlpha(x, y);
    if (g > 0) {
      // blend towards the light accent for the glyph itself
      cr = cr + (LIGHT[0] - cr) * g;
      cg = cg + (LIGHT[1] - cg) * g;
      cb = cb + (LIGHT[2] - cb) * g;
    }

    px[i] = Math.round(cr);
    px[i + 1] = Math.round(cg);
    px[i + 2] = Math.round(cb);
    px[i + 3] = Math.round(bgA * 255);
  }
}

// ---- minimal PNG encoder (RGBA8, filter 0) --------------------------------
const CRC_TABLE = new Int32Array(256).map((_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c;
});

function crc32(buf) {
  let c = -1;
  for (const b of buf) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ -1) >>> 0;
}

function chunk(type, data) {
  const out = Buffer.alloc(8 + data.length + 4);
  out.writeUInt32BE(data.length, 0);
  out.write(type, 4, "ascii");
  data.copy(out, 8);
  out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
  return out;
}

const ihdr = Buffer.alloc(13);
ihdr.writeUInt32BE(S, 0);
ihdr.writeUInt32BE(S, 4);
ihdr[8] = 8; // bit depth
ihdr[9] = 6; // RGBA
const raw = Buffer.alloc(S * (S * 4 + 1));
for (let y = 0; y < S; y++) {
  raw[y * (S * 4 + 1)] = 0; // filter none
  Buffer.from(px.buffer, y * S * 4, S * 4).copy(raw, y * (S * 4 + 1) + 1);
}

const png = Buffer.concat([
  Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
  chunk("IHDR", ihdr),
  chunk("IDAT", deflateSync(raw, { level: 9 })),
  chunk("IEND", Buffer.alloc(0)),
]);

writeFileSync(new URL("../icon-source.png", import.meta.url), png);
console.log("icon-source.png written");
