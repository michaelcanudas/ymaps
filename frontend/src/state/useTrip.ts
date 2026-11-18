import { useCallback, useEffect, useRef, useState } from 'react';
import { planRoute, refreshRoute } from '../api/client';
import { Coordinate, Place, Route } from '../api/types';
import { Preferences, routePreferences } from './usePreferences';

export type RouteState =
  | { status: 'idle' }
  | { status: 'needsOrigin' }
  | { status: 'loading'; previous: Route | null }
  | { status: 'choosing'; places: Place[] }
  | { status: 'ready'; options: Route[]; selected: number }
  | { status: 'error'; message: string };

const refreshIntervalMs = 60000;

type Submission = { destination: string; placeId?: string };

export function selectedRoute(state: RouteState) {
  return state.status === 'ready' ? state.options[state.selected] : null;
}

export function useTrip(currentLocation: Coordinate | null, preferences: Preferences) {
  const [destination, setDestination] = useState('');
  const [pinnedOrigin, setPinnedOrigin] = useState<Coordinate | null>(null);
  const [submission, setSubmission] = useState<Submission | null>(null);
  const [routeState, setRouteState] = useState<RouteState>({ status: 'idle' });
  const [isLive, setIsLive] = useState(false);
  const latestRequestId = useRef(0);
  const currentLocationRef = useRef(currentLocation);
  currentLocationRef.current = currentLocation;

  const origin = pinnedOrigin ?? currentLocation;
  const hasOrigin = origin !== null;
  const preferenceKey = JSON.stringify(routePreferences(preferences) ?? null);

  useEffect(() => {
    if (!submission) return;
    const start = pinnedOrigin ?? currentLocationRef.current;
    if (!start) {
      setRouteState({ status: 'needsOrigin' });
      return;
    }
    const requestId = ++latestRequestId.current;
    const preferenceFields = JSON.parse(preferenceKey);
    setIsLive(false);
    setRouteState((current) => ({ status: 'loading', previous: selectedRoute(current) }));
    planRoute({
      origin: start,
      destination: submission.destination,
      placeId: submission.placeId,
      ...(preferenceFields ? { preferences: preferenceFields } : {}),
    })
      .then((result) => {
        if (requestId !== latestRequestId.current) return;
        if (result.kind === 'route') {
          const { alternatives = [], ...main } = result.route;
          setRouteState({ status: 'ready', options: [main, ...alternatives], selected: 0 });
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
  }, [submission, pinnedOrigin, hasOrigin, preferenceKey]);

  const liveRouteId = isLive ? selectedRoute(routeState)?.id ?? null : null;
  const pinnedOriginRef = useRef(pinnedOrigin);
  pinnedOriginRef.current = pinnedOrigin;

  useEffect(() => {
    if (!liveRouteId) return;
    const timer = setInterval(() => {
      const position = currentLocationRef.current ?? pinnedOriginRef.current;
      if (!position) return;
      const requestId = latestRequestId.current;
      refreshRoute(liveRouteId, position)
        .then((route) => {
          if (requestId !== latestRequestId.current) return;
          if (!route) {
            setSubmission((current) => (current ? { ...current } : current));
            return;
          }
          setRouteState((current) => {
            if (current.status !== 'ready') return current;
            const options = current.options.map((option, index) => (index === current.selected ? route : option));
            return { ...current, options };
          });
        })
        .catch(() => undefined);
    }, refreshIntervalMs);
    return () => clearInterval(timer);
  }, [liveRouteId]);

  const selectOption = useCallback((index: number) => {
    setRouteState((current) =>
      current.status === 'ready' && current.options[index] ? { ...current, selected: index } : current,
    );
  }, []);

  const start = useCallback(() => setIsLive(true), []);
  const end = useCallback(() => setIsLive(false), []);

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
    setIsLive(false);
    setRouteState({ status: 'idle' });
  }, []);

  return {
    destination,
    pinnedOrigin,
    routeState,
    route: selectedRoute(routeState),
    isLive,
    isWaitingForLocation: routeState.status === 'needsOrigin',
    editDestination,
    submit,
    choosePlace,
    pinOrigin: setPinnedOrigin,
    clearTrip,
    selectOption,
    start,
    end,
  };
}

export type Trip = ReturnType<typeof useTrip>;
