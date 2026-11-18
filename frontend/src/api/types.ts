export type Coordinate = { lat: number; lon: number };

export type Entryway = { name: string; coordinate: Coordinate };

export type Place = {
  id: string;
  name: string;
  address?: string;
  coordinate: Coordinate;
  entryway?: Entryway;
  source: 'alias' | 'yale' | 'google';
};

export type RouteRequest = {
  origin: Coordinate;
  destination: string;
  placeId?: string;
};

export type Unresolved = { destination: string; places: Place[] };

export type Point = { id?: string; name?: string; coordinate: Coordinate };

export type Line = { id: string; name: string; color: string; path: string };

export type Vehicle = { id: string; name?: string; coordinate: Coordinate; heading?: number };

type LegBase = {
  from: Point;
  to: Point;
  departAt: string;
  arriveAt: string;
  arriveAtLate: string;
  costCents: number;
  path: string;
};

export type WalkLeg = LegBase & { mode: 'walk' };

export type ShuttleLeg = LegBase & { mode: 'shuttle'; line: Line; stopCount: number; vehicle?: Vehicle };

export type BusLeg = LegBase & {
  mode: 'bus';
  line: Line;
  headsign: string;
  stopCount: number;
  vehicle?: Vehicle;
};

export type BikeLeg = LegBase & { mode: 'bike'; bikesAvailable?: number };

export type VeoLeg = LegBase & {
  mode: 'veo';
  vehicle: Vehicle;
  form: 'scooter' | 'bike';
  rentalUri?: string;
};

export type Leg = WalkLeg | ShuttleLeg | BusLeg | BikeLeg | VeoLeg;

export type Mode = Leg['mode'];

export type Route = {
  id: string;
  place: Place;
  computedAt: string;
  departAt: string;
  arriveAt: string;
  arriveAtLate: string;
  costCents: number;
  legs: Leg[];
};

export type HelloResponse = { message: 'hello' };

export type ApiError = { message: string };
