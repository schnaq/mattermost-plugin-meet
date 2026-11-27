import manifest from 'manifest';

import {get as getPreference} from 'mattermost-redux/selectors/entities/preferences';

import type {PluginRegistry} from './types/mattermost-webapp';

// registerUserSettings stores values in the preference category `pp_${pluginId}` (Mattermost 12.1).
const CATEGORY = `pp_${manifest.id}`;

export type RingPref = 'ring_sound' | 'ring_dialog';

export const getRingPref = (state: Record<string, any>, name: RingPref) => getPreference(state as any, CATEGORY, name, 'on') !== 'off';

// Settings strings are plain text, so the caller resolves them in the user's locale.
export function registerSettings(registry: PluginRegistry, t: (id: string, fallback: string) => string) {
    const options = [{value: 'on', text: t('meet.settings.on', 'On')}, {value: 'off', text: t('meet.settings.off', 'Off')}];
    registry.registerUserSettings({
        id: manifest.id,
        uiName: t('meet.settings.title', 'Huddles'),
        sections: [{
            title: t('meet.settings.title', 'Huddles'),
            settings: [
                {name: 'ring_sound', title: t('meet.settings.ring_sound', 'Ring sound'), type: 'radio', default: 'on', options},
                {name: 'ring_dialog', title: t('meet.settings.ring_dialog', 'Ring dialog'), type: 'radio', default: 'on', options},
            ],
        }],
    } as any);
}
