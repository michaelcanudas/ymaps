import { Leg } from '../api/types';

export function secondsBetween(startIso: string, endIso: string) {
  return Math.max(0, (Date.parse(endIso) - Date.parse(startIso)) / 1000);
}

export function legSeconds(leg: Leg) {
  return secondsBetween(leg.departAt, leg.arriveAt);
}

export function minutesLabel(seconds: number) {
  const minutes = Math.max(1, Math.round(seconds / 60));
  return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)} h ${minutes % 60} min`;
}

export function clockLabel(isoTime: string) {
  return new Date(isoTime).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

export function costLabel(costCents: number) {
  return costCents === 0 ? 'Free' : `$${(costCents / 100).toFixed(2)}`;
}
