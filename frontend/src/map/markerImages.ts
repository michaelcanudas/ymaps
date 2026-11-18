import type { Map as MapLibreMap } from 'maplibre-gl';
import { Mode } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { palette } from '../theme/palette';

const pixelRatio = 2;

const headingStep = 10;
const rotatingSprites = { shuttle: 'yale-shuttle', bus: 'ct-transit-bus' } as const;
const staticSprites = { bike: 'ride-new-haven-bike', veo: 'veo-scooter' } as const;
const dropoffModes: Mode[] = ['bike', 'veo'];

type RotatingMode = keyof typeof rotatingSprites;
type StaticMode = keyof typeof staticSprites;

const headings = Array.from({ length: 360 / headingStep }, (_, i) => i * headingStep);
const pad = (heading: number) => String(heading).padStart(3, '0');

function snapHeading(degrees: number) {
  const normalized = ((degrees % 360) + 360) % 360;
  return (Math.round(normalized / headingStep) * headingStep) % 360;
}

export function vehicleImageId(mode: RotatingMode, heading: number): string;
export function vehicleImageId(mode: StaticMode): string;
export function vehicleImageId(mode: RotatingMode | StaticMode, heading = 0) {
  return mode in rotatingSprites ? `vehicle-${mode}-${pad(snapHeading(heading))}` : `vehicle-${mode}`;
}
export const dropoffImageId = (mode: Mode) => `dropoff-${mode}`;
export const destinationImageId = 'endpoint-destination';

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

async function addPngImage(map: MapLibreMap, id: string, url: string) {
  if (map.hasImage(id)) return;
  const { data } = await map.loadImage(url);
  if (!map.hasImage(id)) map.addImage(id, data, { pixelRatio });
}

export async function registerMarkerImages(map: MapLibreMap) {
  await Promise.all([
    ...(Object.entries(rotatingSprites) as [RotatingMode, string][]).flatMap(([mode, slug]) =>
      headings.map((heading) =>
        addPngImage(map, vehicleImageId(mode, heading), `/vehicles/${slug}/${slug}_${pad(heading)}.png`),
      ),
    ),
    ...(Object.entries(staticSprites) as [StaticMode, string][]).map(([mode, slug]) =>
      addPngImage(map, vehicleImageId(mode), `/vehicles/${slug}/${slug}.png`),
    ),
    ...dropoffModes.map((mode) => addSvgImage(map, dropoffImageId(mode), dropoffSvg(mode))),
    addSvgImage(map, destinationImageId, destinationSvg),
  ]);
}
