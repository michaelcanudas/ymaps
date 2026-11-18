import { ApiError, Coordinate, HelloResponse, Route, RouteRequest, Unresolved } from './types';

const baseUrl = (process.env.EXPO_PUBLIC_YMAPS_URL ?? 'http://localhost:8080').replace(/\/$/, '');

export type PlanResult = { kind: 'route'; route: Route } | { kind: 'unresolved'; unresolved: Unresolved };

async function errorMessage(response: Response) {
  try {
    const body = (await response.json()) as ApiError;
    if (body.message) return body.message;
  } catch {}
  return `The server returned ${response.status}`;
}

export async function sayHello(): Promise<HelloResponse> {
  const response = await fetch(`${baseUrl}/v1/hello`);
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.json();
}

export async function planRoute(request: RouteRequest): Promise<PlanResult> {
  const response = await fetch(`${baseUrl}/v1/routes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  });
  if (response.status === 422) return { kind: 'unresolved', unresolved: await response.json() };
  if (!response.ok) throw new Error(await errorMessage(response));
  return { kind: 'route', route: await response.json() };
}

export async function refreshRoute(routeId: string, position: Coordinate): Promise<Route | null> {
  const query = `lat=${position.lat}&lon=${position.lon}`;
  const response = await fetch(`${baseUrl}/v1/routes/${encodeURIComponent(routeId)}?${query}`);
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.json();
}
