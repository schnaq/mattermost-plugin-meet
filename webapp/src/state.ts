import manifest from 'manifest';
import {useDispatch, useSelector} from 'react-redux';

import {getCurrentUserId} from 'mattermost-redux/selectors/entities/common';
import {getStatusForUserId} from 'mattermost-redux/selectors/entities/users';
import type {DispatchFunc} from 'mattermost-redux/types/actions';

import type {HuddleView, ParticipantView, RingOutcomeView, RingView} from './client';
import {getRingPref} from './settings';

// outcome: my last outgoing ring that was declined or missed, for the caller's toast.
// loads: GET /huddles in flight; recent: huddle_changed events (ended ones too) received meanwhile,
// re-applied on top of the snapshot, which may be older than them.
type PluginState = {huddles: Record<string, HuddleView>; ring?: RingView; outcome?: RingOutcomeView; loaded: boolean; loads: number; recent?: Record<string, HuddleView>};
type RootState = Record<string, any>;

const P = `${manifest.id}_`;
const LOAD_STARTED = `${P}HUDDLES_LOAD_STARTED`;
const LOADED = `${P}HUDDLES_LOADED`;
const CHANGED = `${P}HUDDLE_CHANGED`;
const RING = `${P}RING_RECEIVED`;
const RING_ENDED = `${P}RING_ENDED`;
const RESET = `${P}STATE_RESET`;
const OUTCOME = `${P}RING_OUTCOME`;
const OUTCOME_SEEN = `${P}RING_OUTCOME_SEEN`;

export const huddlesLoadStarted = () => ({type: LOAD_STARTED});

// Without huddles (the load failed) only marks the state loaded, keeping what the websocket delivered.
// stale: a load superseded by a newer one (reconnect) only ends, its snapshot may predate the reset.
export const huddlesLoaded = (huddles?: HuddleView[], stale = false) => ({type: LOADED, huddles, stale});
export const huddleChanged = (huddle: HuddleView) => ({type: CHANGED, huddle});
export const ringReceived = (ring: RingView) => ({type: RING, ring});
export const ringEnded = (ringId: string) => ({type: RING_ENDED, ringId});
export const ringOutcome = (outcome: RingOutcomeView) => ({type: OUTCOME, outcome});
export const ringOutcomeSeen = (ringId: string) => ({type: OUTCOME_SEEN, ringId});
export const stateReset = () => ({type: RESET});

const initial: PluginState = {huddles: {}, loaded: false, loads: 0};

const apply = (huddles: Record<string, HuddleView>, changes: HuddleView[]) => {
    const out = {...huddles};
    for (const h of changes) {
        if (h.ended) {
            delete out[h.id];
        } else {
            out[h.id] = h;
        }
    }
    return out;
};

export function reducer(state: PluginState = initial, action: any): PluginState {
    switch (action.type) {
    case LOAD_STARTED:
        return {...state, loads: state.loads + 1, recent: state.recent ?? {}};
    case LOADED: {
        const loads = Math.max(state.loads - 1, 0);
        const snapshot = action.huddles ? Object.fromEntries((action.huddles as HuddleView[]).map((h) => [h.id, h])) : state.huddles;
        return {...state, loaded: state.loaded || !action.stale, huddles: apply(snapshot, Object.values(state.recent ?? {})), loads, recent: loads ? state.recent : undefined};
    }
    case CHANGED: {
        const h: HuddleView = action.huddle;
        return {...state, huddles: apply(state.huddles, [h]), recent: state.recent && {...state.recent, [h.id]: h}};
    }
    case RING:
        return {...state, ring: action.ring};
    case RING_ENDED:
        return state.ring?.ring_id === action.ringId ? {...state, ring: undefined} : state;
    case OUTCOME:
        return {...state, outcome: action.outcome};
    case OUTCOME_SEEN:
        return state.outcome?.ring_id === action.ringId ? {...state, outcome: undefined} : state;
    case RESET: // loads in flight still finish; events from before the reset belong to the old connection
        return {...initial, loads: state.loads, recent: state.loads ? {} : undefined};
    default:
        return state;
    }
}

const plugin = (state: RootState): PluginState => state[`plugins-${manifest.id}`] ?? initial;

export const getHuddleForChannel = (state: RootState, channelId: string) => {
    const {huddles} = plugin(state);
    for (const id in huddles) {
        if (huddles[id].channel_id === channelId) {
            return huddles[id];
        }
    }
    return undefined;
};
export const getHuddleOfUser = (state: RootState, userId: string) => Object.values(plugin(state).huddles).find((h) => h.participants.some((p) => p.user_id === userId));
export const getMyHuddle = (state: RootState) => getHuddleOfUser(state, getCurrentUserId(state as any));
export const isLoaded = (state: RootState) => plugin(state).loaded;
export const getActiveRing = (state: RootState) => plugin(state).ring;
export const getRingOutcome = (state: RootState) => plugin(state).outcome;

// A ring pops up unless the user turned the dialog off or is dnd/ooo (the server also skips those).
export const isRingVisible = (state: RootState) => {
    const status = getStatusForUserId(state as any, getCurrentUserId(state as any));
    return Boolean(getActiveRing(state)) && getRingPref(state, 'ring_dialog') && status !== 'dnd' && status !== 'ooo';
};

// Joins and invites are refused once the end was requested; an empty conference is still joinable.
export const canJoin = (huddle?: HuddleView): huddle is HuddleView => Boolean(huddle && !huddle.end_requested);

// "Anna, Ben +2" with max names; every name without max.
export const participantNames = (ps: ParticipantView[], max = ps.length) => ps.slice(0, max).map((p) => p.display_name).join(', ') + (ps.length > max ? ` +${ps.length - max}` : '');

// mattermost-redux's types make dispatch accept thunks (the host store runs redux-thunk).
export const useThunkDispatch = (): DispatchFunc => useDispatch();
export const useMe = () => useSelector((s: RootState) => getCurrentUserId(s as any));
