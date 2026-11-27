import type {DispatchFunc} from 'mattermost-redux/types/actions';

import * as client from './client';
import {huddleChanged, huddlesLoadStarted, huddlesLoaded, ringEnded} from './state';

declare global {
    interface Window {
        desktop?: unknown;
    }
}

const isDesktopApp = () => Boolean(window.desktop) || navigator.userAgent.includes('Mattermost');

const openMeet = (url: string) => {
    window.open(url, '_blank', 'noopener');
};

// Desktop app: open after the response (the app hands external URLs to the system browser,
// and an about:blank pre-window would become a plugin popup). Browser: pre-open synchronously
// inside the click handler (popup blockers), navigate after the response, close on error.
async function runAndOpen(getUrl: () => Promise<string | undefined>) {
    if (isDesktopApp()) {
        const url = await getUrl();
        if (url) {
            openMeet(url);
        }
        return;
    }
    const win = window.open('about:blank', '_blank');
    if (win) {
        win.opener = null;
    }
    try {
        const url = await getUrl();
        if (!url) {
            win?.close();
        } else if (win) {
            win.location.href = url;
        } else {
            openMeet(url);
        }
    } catch (e) {
        win?.close();
        throw e;
    }
}

// A promised channel id lets callers (e.g. the DM that must be created first) keep the click gesture.
export const startAndOpen = (channelId: string | Promise<string>) => (dispatch: DispatchFunc) => runAndOpen(async () => {
    const res = await client.startHuddle(await channelId);
    dispatch(huddleChanged(res.huddle));
    return res.meet_url;
});

export const joinAndOpen = (huddleId: string) => () => runAndOpen(async () => (await client.joinHuddle(huddleId)).meet_url);

export const acceptRing = (ringId: string) => (dispatch: DispatchFunc) => runAndOpen(async () => {
    const res = await client.respondRing(ringId, 'accept');
    dispatch(ringEnded(ringId));
    return res.meet_url;
});

export const LOAD_RETRY_MS = 3000;

// Loads the active huddles, retrying once; if that fails too the UI still leaves its neutral
// loading state (the websocket keeps it current from here on).
let latestLoad = 0;

export const loadHuddles = () => async (dispatch: DispatchFunc) => {
    const mine = ++latestLoad;
    dispatch(huddlesLoadStarted()); // events from here on survive the (older) snapshot
    const fetchOnce = () => client.fetchHuddles().catch(() => undefined);
    let huddles = await fetchOnce();
    if (!huddles) {
        await new Promise((r) => setTimeout(r, LOAD_RETRY_MS));
        huddles = await fetchOnce();
    }
    dispatch(mine === latestLoad ? huddlesLoaded(huddles) : huddlesLoaded(undefined, true));
};
