import type { Map as MapLibreMap } from 'maplibre-gl';
import { Mode } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { palette } from '../theme/palette';

const pixelRatio = 2;

const vehicleModes: Mode[] = ['shuttle', 'bus', 'bike', 'veo'];

export const vehicleImageId = (mode: Mode) => `vehicle-${mode}`;
export const headingImageId = (mode: Mode) => `heading-${mode}`;
export const pickupImageId = (mode: Mode) => `pickup-${mode}`;
export const dropoffImageId = (mode: Mode) => `dropoff-${mode}`;
export const destinationImageId = 'endpoint-destination';

function vehicleSvg(mode: Mode) {
  const { color, glyph } = modeAppearance[mode];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 22 22">
    <circle cx="11" cy="11" r="10" fill="${palette.ink}" stroke="${color}" stroke-opacity="0.85" stroke-width="1.2"/>
    <g transform="translate(4.4 4.4) scale(0.55)"><path d="${glyph}" fill="${color}"/></g>
  </svg>`;
}

function headingSvg(mode: Mode) {
  const { color } = modeAppearance[mode];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="30" viewBox="0 0 30 30">
    <path d="M15 1.5 L19 7 L11 7 Z" fill="${color}" fill-opacity="0.9"/>
  </svg>`;
}

function pickupSvg(mode: Mode) {
  const { color } = modeAppearance[mode];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 12 12">
    <rect x="1" y="1" width="10" height="10" rx="3" fill="${color}" fill-opacity="0.8" stroke="${palette.ink}" stroke-width="1.2"/>
  </svg>`;
}

function dropoffSvg(mode: Mode) {
  const { color } = modeAppearance[mode];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 12 12">
    <rect x="1.5" y="1.5" width="9" height="9" rx="3" fill="${palette.ink}" fill-opacity="0.7" stroke="${color}" stroke-opacity="0.8" stroke-width="1.4"/>
  </svg>`;
}

const destinationSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18">
  <rect x="1" y="1" width="16" height="16" rx="2" fill="${palette.textPrimary}"/>
  <rect x="6" y="6" width="6" height="6" fill="${palette.ink}"/>
</svg>`;

function loadSvg(svg: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = reject;
    image.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  });
}

async function addSvgImage(map: MapLibreMap, id: string, svg: string) {
  if (map.hasImage(id)) return;
  const scaled = svg.replace(
    /width="(\d+)" height="(\d+)"/,
    (_, width: string, height: string) =>
      `width="${Number(width) * pixelRatio}" height="${Number(height) * pixelRatio}"`,
  );
  const image = await loadSvg(scaled);
  if (!map.hasImage(id)) map.addImage(id, image, { pixelRatio });
}

export async function registerMarkerImages(map: MapLibreMap) {
  await Promise.all([
    ...vehicleModes.flatMap((mode) => [
      addSvgImage(map, vehicleImageId(mode), vehicleSvg(mode)),
      addSvgImage(map, headingImageId(mode), headingSvg(mode)),
      addSvgImage(map, pickupImageId(mode), pickupSvg(mode)),
      addSvgImage(map, dropoffImageId(mode), dropoffSvg(mode)),
    ]),
    addSvgImage(map, destinationImageId, destinationSvg),
  ]);
}
