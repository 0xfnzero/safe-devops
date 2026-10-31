import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const require = createRequire(join(root, "web/package.json"));
const sharp = require("sharp");
const logo = await readFile(join(root, "assets/brand/logo.svg"), "utf8");
const mark = logo.replace(/<svg[^>]*>/, "").replace(/<\/svg>\s*$/, "").replace(/<title>.*?<\/title>/, "");
const icon = `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
  <title>safe-devops</title>
  <defs>
    <linearGradient id="tile" x1="140" y1="88" x2="820" y2="936" gradientUnits="userSpaceOnUse">
      <stop stop-color="#34373A"/><stop offset=".5" stop-color="#1D1F22"/><stop offset="1" stop-color="#101214"/>
    </linearGradient>
    <linearGradient id="rim" x1="160" y1="88" x2="820" y2="936" gradientUnits="userSpaceOnUse">
      <stop stop-color="#75777A" stop-opacity=".65"/><stop offset=".5" stop-color="#57595C" stop-opacity=".3"/><stop offset="1" stop-color="#090A0B"/>
    </linearGradient>
    <filter id="shadow" x="-20%" y="-20%" width="140%" height="145%"><feDropShadow dx="0" dy="12" stdDeviation="12" flood-opacity=".22"/></filter>
  </defs>
  <rect x="88" y="88" width="848" height="848" rx="190" fill="url(#tile)" filter="url(#shadow)"/>
  <rect x="89.5" y="89.5" width="845" height="845" rx="188.5" fill="none" stroke="url(#rim)" stroke-width="3"/>
  <g transform="translate(164 150) scale(1.359375)">${mark}</g>
</svg>
`;

async function save(relative, bytes) {
  const path = join(root, relative);
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, bytes);
}
const pngs = new Map();
for (const size of [16, 24, 32, 48, 64, 128, 180, 256, 512, 1024]) {
  pngs.set(size, await sharp(Buffer.from(icon)).resize(size, size).png().toBuffer());
}

// Modern Windows ICO stores one PNG for each size, including a 256px image.
function ico(sizes) {
  const header = Buffer.alloc(6 + 16 * sizes.length);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(sizes.length, 4);
  let offset = header.length;
  sizes.forEach((size, i) => {
    const at = 6 + 16 * i;
    const data = pngs.get(size);
    header[at] = header[at + 1] = size === 256 ? 0 : size;
    header.writeUInt16LE(1, at + 4);
    header.writeUInt16LE(32, at + 6);
    header.writeUInt32LE(data.length, at + 8);
    header.writeUInt32LE(offset, at + 12);
    offset += data.length;
  });
  return Buffer.concat([header, ...sizes.map(size => pngs.get(size))]);
}

// macOS ICNS includes standard and Retina PNG representations.
const chunks = Object.entries({icp4:16, icp5:32, icp6:64, ic07:128, ic08:256, ic09:512, ic10:1024, ic11:32, ic12:64, ic13:256, ic14:512}).map(([type, size]) => {
  const data = pngs.get(size);
  const header = Buffer.alloc(8);
  header.write(type);
  header.writeUInt32BE(data.length + 8, 4);
  return Buffer.concat([header, data]);
});
const icnsHeader = Buffer.alloc(8);
icnsHeader.write("icns");
icnsHeader.writeUInt32BE(8 + chunks.reduce((sum, chunk) => sum + chunk.length, 0), 4);

await save("assets/brand/app-icon.svg", icon);
await save("assets/brand/app-icon.png", pngs.get(1024));
await save("desktop/assets/icon.png", pngs.get(512));
await save("desktop/assets/icon.ico", ico([16, 24, 32, 48, 64, 128, 256]));
await save("desktop/assets/icon.icns", Buffer.concat([icnsHeader, ...chunks]));
for (const size of [16, 32, 48, 64, 128, 256, 512]) {
  await save(`desktop/assets/icons/${size}x${size}.png`, pngs.get(size));
}
await save("desktop/assets/logo.svg", logo);
await save("web/public/brand/logo.svg", logo);
await save("web/public/brand/icon.png", pngs.get(128));
await save("web/src/app/icon.svg", icon);
await save("web/src/app/favicon.ico", ico([16, 32, 48]));
// iOS applies its own corner mask; flatten to an opaque tile.
const apple = await sharp(Buffer.from(icon)).extract({left:88, top:88, width:848, height:848}).flatten({background:"#1D1F22"}).resize(180, 180).png().toBuffer();
await save("web/src/app/apple-icon.png", apple);
console.log("Generated safe-devops icons: macOS ICNS, Windows ICO, Linux PNG, Web and launcher assets.");
