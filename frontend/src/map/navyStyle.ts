import type { ExpressionSpecification, LineLayerSpecification, StyleSpecification } from 'maplibre-gl';
import { palette } from '../theme/palette';

const regularFont = ['Noto Sans Regular'];
const boldFont = ['Noto Sans Bold'];

function roadWidth(base: number): ExpressionSpecification {
  return [
    'interpolate',
    ['exponential', 1.6],
    ['zoom'],
    12,
    base * 0.4,
    15,
    base * 1.4,
    18,
    base * 6,
  ];
}

function roadLayer(
  id: string,
  classes: string[],
  color: string,
  base: number,
  minzoom = 0,
): LineLayerSpecification {
  return {
    id,
    type: 'line',
    source: 'openmaptiles',
    'source-layer': 'transportation',
    minzoom,
    filter: ['all', ['in', ['get', 'class'], ['literal', classes]], ['!=', ['get', 'brunnel'], 'tunnel']],
    layout: { 'line-cap': 'round', 'line-join': 'round' },
    paint: { 'line-color': color, 'line-width': roadWidth(base) },
  };
}

export const navyStyle: StyleSpecification = {
  version: 8,
  glyphs: 'https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf',
  sources: {
    openmaptiles: { type: 'vector', url: 'https://tiles.openfreemap.org/planet' },
  },
  layers: [
    { id: 'background', type: 'background', paint: { 'background-color': palette.mapBackground } },
    {
      id: 'park',
      type: 'fill',
      source: 'openmaptiles',
      'source-layer': 'park',
      paint: { 'fill-color': palette.park },
    },
    {
      id: 'landcover-green',
      type: 'fill',
      source: 'openmaptiles',
      'source-layer': 'landcover',
      filter: ['in', ['get', 'class'], ['literal', ['grass', 'wood']]],
      paint: { 'fill-color': palette.park, 'fill-opacity': 0.7 },
    },
    {
      id: 'landuse-campus',
      type: 'fill',
      source: 'openmaptiles',
      'source-layer': 'landuse',
      filter: ['in', ['get', 'class'], ['literal', ['university', 'college', 'school', 'hospital']]],
      paint: { 'fill-color': '#112038' },
    },
    {
      id: 'water',
      type: 'fill',
      source: 'openmaptiles',
      'source-layer': 'water',
      paint: { 'fill-color': palette.water },
    },
    {
      id: 'waterway',
      type: 'line',
      source: 'openmaptiles',
      'source-layer': 'waterway',
      paint: { 'line-color': palette.water, 'line-width': 1.2 },
    },
    {
      id: 'building',
      type: 'fill',
      source: 'openmaptiles',
      'source-layer': 'building',
      minzoom: 14,
      paint: {
        'fill-color': palette.building,
        'fill-outline-color': palette.buildingOutline,
        'fill-opacity': ['interpolate', ['linear'], ['zoom'], 14, 0, 15.5, 1],
      },
    },
    roadLayer('road-path', ['path', 'track'], palette.roadMinor, 0.5, 15),
    roadLayer('road-service', ['service'], palette.roadMinor, 0.7, 14),
    roadLayer('road-minor', ['minor'], palette.roadMinor, 1.1, 12),
    roadLayer('road-tertiary', ['tertiary'], palette.roadMajor, 1.4),
    roadLayer('road-secondary', ['secondary'], palette.roadMajor, 1.7),
    roadLayer('road-primary', ['primary', 'trunk'], palette.roadMajor, 2),
    roadLayer('road-highway', ['motorway'], palette.roadHighway, 2.4),
    {
      id: 'rail',
      type: 'line',
      source: 'openmaptiles',
      'source-layer': 'transportation',
      filter: ['==', ['get', 'class'], 'rail'],
      paint: { 'line-color': palette.rail, 'line-width': 1, 'line-dasharray': [3, 3] },
    },
    {
      id: 'boundary',
      type: 'line',
      source: 'openmaptiles',
      'source-layer': 'boundary',
      filter: ['<=', ['get', 'admin_level'], 4],
      paint: { 'line-color': palette.boundary, 'line-width': 1, 'line-dasharray': [2, 2] },
    },
    {
      id: 'road-label',
      type: 'symbol',
      source: 'openmaptiles',
      'source-layer': 'transportation_name',
      minzoom: 14,
      filter: ['in', ['get', 'class'], ['literal', ['primary', 'secondary', 'tertiary', 'minor', 'trunk']]],
      layout: {
        'symbol-placement': 'line',
        'text-field': ['get', 'name'],
        'text-font': regularFont,
        'text-size': ['interpolate', ['linear'], ['zoom'], 14, 9, 18, 12],
        'text-letter-spacing': 0.02,
      },
      paint: {
        'text-color': palette.mapLabelFaint,
        'text-halo-color': palette.mapBackground,
        'text-halo-width': 1.2,
      },
    },
    {
      id: 'water-label',
      type: 'symbol',
      source: 'openmaptiles',
      'source-layer': 'water_name',
      layout: { 'text-field': ['get', 'name'], 'text-font': regularFont, 'text-size': 10 },
      paint: { 'text-color': palette.mapLabelFaint, 'text-halo-color': palette.water, 'text-halo-width': 1 },
    },
    {
      id: 'place-neighbourhood',
      type: 'symbol',
      source: 'openmaptiles',
      'source-layer': 'place',
      minzoom: 13,
      maxzoom: 17,
      filter: ['in', ['get', 'class'], ['literal', ['neighbourhood', 'suburb', 'quarter']]],
      layout: {
        'text-field': ['get', 'name'],
        'text-font': regularFont,
        'text-size': 10,
        'text-transform': 'uppercase',
        'text-letter-spacing': 0.12,
      },
      paint: {
        'text-color': palette.mapLabelFaint,
        'text-halo-color': palette.mapBackground,
        'text-halo-width': 1.2,
      },
    },
    {
      id: 'place-city',
      type: 'symbol',
      source: 'openmaptiles',
      'source-layer': 'place',
      maxzoom: 14,
      filter: ['in', ['get', 'class'], ['literal', ['city', 'town']]],
      layout: { 'text-field': ['get', 'name'], 'text-font': boldFont, 'text-size': 13 },
      paint: {
        'text-color': palette.mapLabel,
        'text-halo-color': palette.mapBackground,
        'text-halo-width': 1.4,
      },
    },
  ],
};

export const firstLabelLayerId = 'road-label';
