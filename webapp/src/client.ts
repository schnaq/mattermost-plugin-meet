import manifest from 'manifest';

import {Client4} from 'mattermost-redux/client';

export type ParticipantView = {user_id: string; /* '' for guests */ display_name: string; state: 'joining' | 'confirmed'};
// ending: the conference is active but empty (still joinable); end_requested: joins are refused.
export type HuddleView = {id: string; channel_id: string; starter_id: string; started_at: number; ending: boolean; end_requested: boolean; participants: ParticipantView[]; ended?: boolean};
export type RingView = {ring_id: string; huddle_id: string; channel_id: string; from: string};
export type RingOutcomeView = {ring_id: string; state: 'declined' | 'missed'; to: string}; // caller only; no quick replies

const base = () => `${Client4.getUrl()}/plugins/${manifest.id}/api/v1`;

// Throws an Error carrying the server's localized {error} text.
async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
    const options = Client4.getOptions({method, body: body === undefined ? undefined : JSON.stringify(body)});
    const res = await fetch(base() + path, options);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
        throw new Error(data?.error || res.statusText);
    }
    return data as T;
}

export const fetchHuddles = () => request<HuddleView[]>('/huddles');
export const startHuddle = (channelId: string) => request<{huddle: HuddleView; meet_url: string; already_running?: boolean}>('/huddles', 'POST', {channel_id: channelId});
export const joinHuddle = (id: string) => request<{meet_url: string}>(`/huddles/${id}/join`, 'POST');
export const inviteToHuddle = (id: string, userIds: string[]) => request<void>(`/huddles/${id}/invite`, 'POST', {user_ids: userIds});
export const endHuddle = (id: string) => request<void>(`/huddles/${id}/end`, 'POST');
export const respondRing = (id: string, action: 'accept' | 'decline' | 'quickreply', text?: string) => request<{meet_url?: string}>(`/rings/${id}`, 'POST', {action, text});
