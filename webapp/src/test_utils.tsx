import manifest from 'manifest';
import type React from 'react';
import {IntlProvider} from 'react-intl';
import {Provider} from 'react-redux';
import {applyMiddleware, createStore} from 'redux';

import type {HuddleView} from './client';
import {reducer} from './state';

export const huddle = (over: Partial<HuddleView> = {}): HuddleView => ({
    id: 'h1',
    channel_id: 'c1',
    starter_id: 'me',
    started_at: Date.now(),
    ending: false,
    end_requested: false,
    participants: [{user_id: 'me', display_name: 'Me', state: 'confirmed'}],
    ...over,
});

export const makeState = (huddles: HuddleView[] = [], extra: Record<string, unknown> = {}) => ({
    entities: {
        users: {currentUserId: 'me', profiles: {}, profilesInChannel: {}},
        channels: {currentChannelId: 'c1'},
        general: {config: {}},
        preferences: {myPreferences: {}},
    },
    [`plugins-${manifest.id}`]: {loaded: true, huddles: Object.fromEntries(huddles.map((h) => [h.id, h]))},
    ...extra,
});

// Minimal thunk support, the host store has redux-thunk.
const thunk = (api: any) => (next: any) => (action: any) => (typeof action === 'function' ? action(api.dispatch, api.getState) : next(action));

// The plugin slice runs through the real reducer (e.g. a closed ring or a seen outcome clears);
// the host's entities stay as given.
export function wrap(state: ReturnType<typeof makeState>, children: React.ReactNode) {
    const key = `plugins-${manifest.id}`;
    const root = (s: any = state, action: any) => ({...s, [key]: reducer(s[key], action)});
    const store = createStore(root, applyMiddleware(thunk as any));
    return (
        <Provider store={store}>
            <IntlProvider
                locale='en'
                defaultLocale='en'
            >
                {children}
            </IntlProvider>
        </Provider>
    );
}

// State for ring tests: a ringing call from "caller", own status, and the plugin's user preferences.
export function ringState(o: {status?: string; sound?: string; dialog?: string; ring?: boolean; huddles?: HuddleView[]} = {}) {
    const category = `pp_${manifest.id}`;
    const prefs: Record<string, unknown> = {};
    for (const [name, value] of [['ring_sound', o.sound], ['ring_dialog', o.dialog]] as const) {
        if (value !== undefined) {
            prefs[`${category}--${name}`] = {category, name, value};
        }
    }
    const base = makeState(o.huddles ?? []);
    return {
        ...base,
        entities: {
            ...base.entities,
            users: {
                ...base.entities.users,
                profiles: {caller: {id: 'caller', username: 'anna', first_name: '', last_name: ''}},
                statuses: {me: o.status ?? 'online'},
            },
            channels: {currentChannelId: 'c1', channels: {c1: {id: 'c1', name: 'town', display_name: 'Town Square', type: 'O'}}},
            preferences: {myPreferences: prefs},
            general: {config: {}},
            teams: {currentTeamId: 't1', teams: {t1: {id: 't1', name: 'team'}}},
        },
        [`plugins-${manifest.id}`]: {
            loaded: true,
            huddles: Object.fromEntries((o.huddles ?? []).map((h) => [h.id, h])),
            ring: o.ring === false ? undefined : {ring_id: 'r1', huddle_id: 'h1', channel_id: 'c1', from: 'caller'},
        },
    };
}
