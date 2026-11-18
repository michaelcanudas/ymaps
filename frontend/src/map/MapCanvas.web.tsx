import 'maplibre-gl/dist/maplibre-gl.css';
import './mapChrome.css';
import maplibregl, { GeoJSONSource, LngLatBounds, Map as MapLibreMap } from 'maplibre-gl';
import type { Feature, FeatureCollection } from 'geojson';
import { useEffect, useRef, useState } from 'react';
import { decodePolyline } from '../api/polyline';
import { Coordinate, Leg, Route } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { palette } from '../theme/palette';
import { campusCenter, MapCanvasProps } from './mapProps';
import {
  destinationImageId,
  dropoffImageId,
  headingImageId,
  pickupImageId,
  registerMarkerImages,
  vehicleImageId,
} from './markerImages';
import { firstLabelLayerId, navyStyle } from './navyStyle';

const emptyCollection: FeatureCollection = { type: 'FeatureCollection', features: [] };

function collection(features: Feature[]): FeatureCollection {
  return { type: 'FeatureCollection', features };
}

function point(coordinate: Coordinate, properties: Record<string, unknown>): Feature {
  return {
    type: 'Feature',
    geometry: { type: 'Point', coordinates: [coordinate.lon, coordinate.lat] },
    properties,
  };
}

function line(path: [number, number][], properties: Record<string, unknown>): Feature {
  return { type: 'Feature', geometry: { type: 'LineString', coordinates: path }, properties };
}

function legPath(leg: Leg): [number, number][] {
  const decoded = decodePolyline(leg.path);
  if (decoded.length >= 2) return decoded;
  return [
    [leg.from.coordinate.lon, leg.from.coordinate.lat],
    [leg.to.coordinate.lon, leg.to.coordinate.lat],
  ];
}

function transitLineCollection(route: Route | null) {
  if (!route) return emptyCollection;
  return collection(
    route.legs.flatMap((leg) =>
      leg.mode === 'shuttle' || leg.mode === 'bus'
        ? [line(decodePolyline(leg.line.path), { color: `#${leg.line.color}` })]
        : [],
    ),
  );
}

function legCollection(route: Route | null) {
  if (!route) return emptyCollection;
  return collection(
    route.legs.map((leg) => line(legPath(leg), { mode: leg.mode, color: modeAppearance[leg.mode].color })),
  );
}

function markerCollection(route: Route | null) {
  if (!route) return emptyCollection;
  return collection(
    route.legs.flatMap((leg): Feature[] => {
      switch (leg.mode) {
        case 'shuttle':
        case 'bus':
          return leg.vehicle
            ? [
                point(leg.vehicle.coordinate, {
                  icon: vehicleImageId(leg.mode),
                  heading: headingImageId(leg.mode),
                  hasHeading: leg.vehicle.heading !== undefined,
                  headingDegrees: leg.vehicle.heading ?? 0,
                }),
              ]
            : [];
        case 'bike':
          return [
            point(leg.from.coordinate, { icon: pickupImageId('bike'), hasHeading: false }),
            point(leg.to.coordinate, { icon: dropoffImageId('bike'), hasHeading: false }),
          ];
        case 'veo':
          return [
            point(leg.vehicle.coordinate, { icon: vehicleImageId('veo'), hasHeading: false }),
            point(leg.to.coordinate, { icon: dropoffImageId('veo'), hasHeading: false }),
          ];
        default:
          return [];
      }
    }),
  );
}

function endpointCollection(route: Route | null, pinnedOrigin: Coordinate | null) {
  if (!route || route.legs.length === 0) {
    return pinnedOrigin ? collection([point(pinnedOrigin, { kind: 'origin' })]) : emptyCollection;
  }
  return collection([
    point(route.legs[0].from.coordinate, { kind: 'origin' }),
    point(route.legs[route.legs.length - 1].to.coordinate, { kind: 'destination', icon: destinationImageId }),
  ]);
}

function addOverlayLayers(map: MapLibreMap) {
  ['transit-lines', 'legs', 'markers', 'endpoints', 'current-location'].forEach((sourceId) =>
    map.addSource(sourceId, { type: 'geojson', data: emptyCollection }),
  );

  map.addLayer(
    {
      id: 'transit-lines',
      type: 'line',
      source: 'transit-lines',
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': ['get', 'color'], 'line-width': 3, 'line-opacity': 0.22 },
    },
    firstLabelLayerId,
  );
  map.addLayer(
    {
      id: 'leg-casing',
      type: 'line',
      source: 'legs',
      filter: ['!=', ['get', 'mode'], 'walk'],
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': palette.ink, 'line-width': 8, 'line-opacity': 0.9 },
    },
    firstLabelLayerId,
  );
  map.addLayer(
    {
      id: 'leg-ride',
      type: 'line',
      source: 'legs',
      filter: ['!=', ['get', 'mode'], 'walk'],
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': ['get', 'color'], 'line-width': 4 },
    },
    firstLabelLayerId,
  );
  map.addLayer(
    {
      id: 'leg-walk',
      type: 'line',
      source: 'legs',
      filter: ['==', ['get', 'mode'], 'walk'],
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': ['get', 'color'], 'line-width': 3.5, 'line-dasharray': [0.1, 2] },
    },
    firstLabelLayerId,
  );

  map.addLayer({
    id: 'marker-heading',
    type: 'symbol',
    source: 'markers',
    filter: ['get', 'hasHeading'],
    layout: {
      'icon-image': ['get', 'heading'],
      'icon-rotate': ['get', 'headingDegrees'],
      'icon-rotation-alignment': 'map',
      'icon-allow-overlap': true,
    },
  });
  map.addLayer({
    id: 'markers',
    type: 'symbol',
    source: 'markers',
    layout: { 'icon-image': ['get', 'icon'], 'icon-allow-overlap': true },
  });

  map.addLayer({
    id: 'origin',
    type: 'circle',
    source: 'endpoints',
    filter: ['==', ['get', 'kind'], 'origin'],
    paint: {
      'circle-radius': 6,
      'circle-color': palette.ink,
      'circle-stroke-color': palette.textPrimary,
      'circle-stroke-width': 3,
    },
  });
  map.addLayer({
    id: 'destination',
    type: 'symbol',
    source: 'endpoints',
    filter: ['==', ['get', 'kind'], 'destination'],
    layout: { 'icon-image': ['get', 'icon'], 'icon-allow-overlap': true },
  });

  map.addLayer({
    id: 'current-location-halo',
    type: 'circle',
    source: 'current-location',
    paint: { 'circle-radius': 16, 'circle-color': palette.accent, 'circle-opacity': 0.14 },
  });
  map.addLayer({
    id: 'current-location-dot',
    type: 'circle',
    source: 'current-location',
    paint: {
      'circle-radius': 6,
      'circle-color': palette.accent,
      'circle-stroke-color': palette.ink,
      'circle-stroke-width': 2,
    },
  });
}

function setSourceData(map: MapLibreMap, sourceId: string, data: FeatureCollection) {
  (map.getSource(sourceId) as GeoJSONSource | undefined)?.setData(data);
}

export function MapCanvas({ route, currentLocation, pinnedOrigin, insets, onMapPress }: MapCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MapLibreMap | null>(null);
  const onMapPressRef = useRef(onMapPress);
  onMapPressRef.current = onMapPress;
  const routeRef = useRef(route);
  routeRef.current = route;
  const [isReady, setIsReady] = useState(false);

  useEffect(() => {
    if (!containerRef.current) return;
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: navyStyle,
      center: [campusCenter.lon, campusCenter.lat],
      zoom: 15,
      pitch: 0,
      dragRotate: false,
      attributionControl: { compact: true },
      fadeDuration: 0,
    });
    map.touchZoomRotate.disableRotation();
    map.on('click', (event) => onMapPressRef.current?.({ lat: event.lngLat.lat, lon: event.lngLat.lng }));
    mapRef.current = map;
    map.on('load', async () => {
      await registerMarkerImages(map);
      addOverlayLayers(map);
      setIsReady(true);
    });
    return () => {
      map.remove();
      mapRef.current = null;
      setIsReady(false);
    };
  }, []);

  useEffect(() => {
    const canvas = containerRef.current?.querySelector<HTMLElement>('.maplibregl-canvas');
    if (canvas) canvas.style.cursor = onMapPress ? 'crosshair' : '';
  }, [isReady, onMapPress]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !isReady) return;
    setSourceData(map, 'transit-lines', transitLineCollection(route));
    setSourceData(map, 'legs', legCollection(route));
    setSourceData(map, 'markers', markerCollection(route));
    setSourceData(map, 'endpoints', endpointCollection(route, pinnedOrigin));
  }, [isReady, route, pinnedOrigin]);

  const routeId = route?.id ?? null;

  useEffect(() => {
    const map = mapRef.current;
    const current = routeRef.current;
    if (!map || !isReady || !current || current.legs.length === 0) return;
    const bounds = new LngLatBounds();
    current.legs.forEach((leg) => legPath(leg).forEach((position) => bounds.extend(position)));
    map.fitBounds(bounds, {
      padding: {
        top: insets.top + 48,
        right: insets.right + 48,
        bottom: insets.bottom + 48,
        left: insets.left + 48,
      },
      maxZoom: 17,
      duration: 450,
    });
  }, [isReady, routeId, insets.top, insets.right, insets.bottom, insets.left]);

  useEffect(() => {
    const corner = containerRef.current?.querySelector<HTMLElement>('.maplibregl-ctrl-bottom-right');
    if (corner) corner.style.bottom = `${insets.bottom}px`;
  }, [isReady, insets.bottom]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !isReady) return;
    setSourceData(
      map,
      'current-location',
      currentLocation ? collection([point(currentLocation, {})]) : emptyCollection,
    );
  }, [isReady, currentLocation]);

  return <div ref={containerRef} style={{ position: 'absolute', inset: 0, background: palette.mapBackground }} />;
}
