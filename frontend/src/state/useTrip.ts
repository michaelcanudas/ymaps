import { useCallback, useEffect, useRef, useState } from 'react';
import { planRoute, refreshRoute } from '../api/client';
import { Coordinate, Place, Route } from '../api/types';

export type RouteState =
  | { status: 'idle' }
  | { status: 'needsOrigin' }
  | { status: 'loading'; previous: Route | null }
  | { status: 'choosing'; places: Place[] }
  | { status: 'ready'; route: Route }
  | { status: 'error'; message: string };

const refreshIntervalMs = 30000;

type Submission = { destination: string; placeId?: string };

export function useTrip(currentLocation: Coordinate | null) {
  const [destination, setDestination] = useState('');
  const [pinnedOrigin, setPinnedOrigin] = useState<Coordinate | null>(null);
  const [submission, setSubmission] = useState<Submission | null>(null);
  const [routeState, setRouteState] = useState<RouteState>({ status: 'idle' });
  const latestRequestId = useRef(0);
  const currentLocationRef = useRef(currentLocation);
  currentLocationRef.current = currentLocation;

  const origin = pinnedOrigin ?? currentLocation;
  const hasOrigin = origin !== null;

  useEffect(() => {
    if (!submission) return;
    const start = pinnedOrigin ?? currentLocationRef.current;
    if (!start) {
      setRouteState({ status: 'needsOrigin' });
      return;
    }
    const requestId = ++latestRequestId.current;
    setRouteState((current) => ({
      status: 'loading',
      previous: current.status === 'ready' ? current.route : null,
    }));
    planRoute({ origin: start, destination: submission.destination, placeId: submission.placeId })
      .then((result) => {
        if (requestId !== latestRequestId.current) return;
        if (result.kind === 'route') {
          setRouteState({ status: 'ready', route: result.route });
        } else if (result.unresolved.places.length > 0) {
          setRouteState({ status: 'choosing', places: result.unresolved.places });
        } else {
          setRouteState({ status: 'error', message: `Nothing matched “${submission.destination}”` });
        }
      })
      .catch((error: unknown) => {
        if (requestId !== latestRequestId.current) return;
        setRouteState({
          status: 'error',
          message: error instanceof Error ? error.message : 'No route right now',
        });
      });
  }, [submission, pinnedOrigin, hasOrigin]);

  const readyRouteId = routeState.status === 'ready' ? routeState.route.id : null;

  useEffect(() => {
    if (!readyRouteId || pinnedOrigin) return;
    const timer = setInterval(() => {
      const position = currentLocationRef.current;
      if (!position) return;
      const requestId = latestRequestId.current;
      refreshRoute(readyRouteId, position)
        .then((route) => {
          if (requestId !== latestRequestId.current) return;
          if (route) setRouteState({ status: 'ready', route });
          else setSubmission((current) => (current ? { ...current } : current));
        })
        .catch(() => undefined);
    }, refreshIntervalMs);
    return () => clearInterval(timer);
  }, [readyRouteId, pinnedOrigin]);

  const submit = useCallback(() => {
    const text = destination.trim();
    if (text) setSubmission({ destination: text });
  }, [destination]);

  const choosePlace = useCallback(
    (place: Place) => {
      setDestination(place.name);
      setSubmission({ destination: destination.trim() || place.name, placeId: place.id });
    },
    [destination],
  );

  const editDestination = useCallback((text: string) => setDestination(text), []);

  const clearTrip = useCallback(() => {
    latestRequestId.current += 1;
    setDestination('');
    setSubmission(null);
    setPinnedOrigin(null);
    setRouteState({ status: 'idle' });
  }, []);

  return {
    destination,
    pinnedOrigin,
    routeState,
    isWaitingForLocation: routeState.status === 'needsOrigin',
    editDestination,
    submit,
    choosePlace,
    pinOrigin: setPinnedOrigin,
    clearTrip,
  };
}

export type Trip = ReturnType<typeof useTrip>;
