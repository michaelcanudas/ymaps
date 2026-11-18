import { useEffect, useState } from 'react';
import { Coordinate } from '../api/types';

export type LocationState =
  | { status: 'locating' }
  | { status: 'located'; coordinate: Coordinate }
  | { status: 'unavailable' };

export function useCurrentLocation() {
  const [location, setLocation] = useState<LocationState>({ status: 'locating' });

  useEffect(() => {
    const geolocation = globalThis.navigator?.geolocation;
    if (!geolocation) {
      setLocation({ status: 'unavailable' });
      return;
    }
    const watchId = geolocation.watchPosition(
      (position) =>
        setLocation({
          status: 'located',
          coordinate: { lat: position.coords.latitude, lon: position.coords.longitude },
        }),
      () => setLocation((current) => (current.status === 'located' ? current : { status: 'unavailable' })),
      { enableHighAccuracy: true, maximumAge: 15000, timeout: 8000 },
    );
    return () => geolocation.clearWatch(watchId);
  }, []);

  return location;
}
