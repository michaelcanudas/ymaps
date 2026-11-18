import { Mode } from '../api/types';

export type ModeAppearance = {
  label: string;
  color: string;
  glyph: string;
};

export const modeAppearance: Record<Mode, ModeAppearance> = {
  walk: {
    label: 'Walk',
    color: '#A9B6C8',
    glyph:
      'M13 4.5a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0ZM10.2 7.6l-2.4 1.6-.9 3.2 1.2.4.7-2.5 1.1-.7-.9 4.3-1.8 4.1 1.2.6 1.9-4 1.1 1.5.4 3.4h1.3l-.5-4-1.4-2 .5-2.4.6 1.4 2.1 1 .5-1.1-1.8-.9-1-2.4a1.6 1.6 0 0 0-1.7-.9l-1.2.4Z',
  },
  bike: {
    label: 'Ride New Haven',
    color: '#5FC8A8',
    glyph:
      'M6 18a3.2 3.2 0 1 1 0-6.4A3.2 3.2 0 0 1 6 18Zm0-1.2a2 2 0 1 0 0-4 2 2 0 0 0 0 4ZM18 18a3.2 3.2 0 1 1 0-6.4 3.2 3.2 0 0 1 0 6.4Zm0-1.2a2 2 0 1 0 0-4 2 2 0 0 0 0 4ZM9.3 8h3.2v1.2h-1.6l1 2h3.3l-1-2.6h-1.4V7.4h2.2l2.6 7-1.1.4-1.1-2.9-2.7 3.8h-2.6l-2.4-4.9-1.6 3.4-1.1-.5 2.3-4.7Zm3.1 3.9 1 2.1 1.5-2.1h-2.5Z',
  },
  veo: {
    label: 'Veo',
    color: '#9BCB7A',
    glyph:
      'M14.5 5h2.7v1.2h-1.8l1.8 8.3a2.8 2.8 0 1 1-1.2.3l-.3-1.5-4.3 3.9H8.6a2.8 2.8 0 1 1 0-1.2h2.3l4.1-3.8L14.5 5ZM6 17.9a1.6 1.6 0 1 0 0-3.2 1.6 1.6 0 0 0 0 3.2Zm11.6 0a1.6 1.6 0 1 0 0-3.2 1.6 1.6 0 0 0 0 3.2Z',
  },
  shuttle: {
    label: 'Yale Shuttle',
    color: '#6EA8FF',
    glyph:
      'M6.5 5h11A1.5 1.5 0 0 1 19 6.5V16a1 1 0 0 1-1 1v1.5h-2V17H8v1.5H6V17a1 1 0 0 1-1-1V6.5A1.5 1.5 0 0 1 6.5 5Zm.3 1.8v4.4h10.4V6.8H6.8Zm1 6.4a1 1 0 1 0 0 2 1 1 0 0 0 0-2Zm8.4 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2Z',
  },
  bus: {
    label: 'CT Transit',
    color: '#E9B872',
    glyph:
      'M7 4h10a2 2 0 0 1 2 2v10.5a1 1 0 0 1-1 1V19h-2v-1.5H8V19H6v-1.5a1 1 0 0 1-1-1V6a2 2 0 0 1 2-2Zm-.2 2.2v5h10.4v-5H6.8Zm2.2-1.4v.8h6v-.8H9Zm-1.2 8.6a1 1 0 1 0 0 2 1 1 0 0 0 0-2Zm8.4 0a1 1 0 1 0 0 2 1 1 0 0 0 0-2Z',
  },
};
