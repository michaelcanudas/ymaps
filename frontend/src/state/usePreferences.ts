import { useCallback, useEffect, useState } from 'react';
import { AvoidableMode, avoidableModes, RoutePreferences } from '../api/types';

export type Preferences = { modes: AvoidableMode[] };

const storageKey = 'ymaps.preferences';

const defaultPreferences: Preferences = { modes: avoidableModes };

function readStored(): Preferences {
  try {
    const stored = globalThis.localStorage?.getItem(storageKey);
    if (!stored) return defaultPreferences;
    const parsed = JSON.parse(stored) as Partial<Preferences>;
    const modes = Array.isArray(parsed.modes)
      ? avoidableModes.filter((mode) => parsed.modes?.includes(mode))
      : avoidableModes;
    return { modes };
  } catch {
    return defaultPreferences;
  }
}

function writeStored(preferences: Preferences) {
  try {
    globalThis.localStorage?.setItem(storageKey, JSON.stringify(preferences));
  } catch {}
}

export function routePreferences(preferences: Preferences): RoutePreferences | undefined {
  const avoidModes = avoidableModes.filter((mode) => !preferences.modes.includes(mode));
  return avoidModes.length > 0 ? { avoidModes } : undefined;
}

export function usePreferences() {
  const [preferences, setPreferences] = useState<Preferences>(defaultPreferences);

  useEffect(() => setPreferences(readStored()), []);

  const update = useCallback((change: (current: Preferences) => Preferences) => {
    setPreferences((current) => {
      const next = change(current);
      writeStored(next);
      return next;
    });
  }, []);

  const toggleMode = useCallback(
    (mode: AvoidableMode) =>
      update((current) => ({
        ...current,
        modes: avoidableModes.filter((existing) =>
          existing === mode ? !current.modes.includes(mode) : current.modes.includes(existing),
        ),
      })),
    [update],
  );

  return { preferences, toggleMode };
}
