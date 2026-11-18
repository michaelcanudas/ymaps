import { Coordinate, Route } from '../api/types';

export type MapInsets = { top: number; right: number; bottom: number; left: number };

export type MapCanvasProps = {
  route: Route | null;
  currentLocation: Coordinate | null;
  pinnedOrigin: Coordinate | null;
  insets: MapInsets;
  onMapPress?: (coordinate: Coordinate) => void;
};

export const campusCenter: Coordinate = { lat: 41.3098, lon: -72.9275 };
