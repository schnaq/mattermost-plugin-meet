import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import {getProfilesInChannel} from 'mattermost-redux/actions/users';

import {HuddleCard} from './huddle_card';

import * as client from '../client';
import {huddle, makeState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/users', () => ({getProfilesInChannel: jest.fn(() => () => Promise.resolve())}));
jest.mock('../client');
jest.mock('../actions', () => ({joinAndOpen: jest.fn(() => () => Promise.resolve())}));

const post = (props: Record<string, unknown> = {}, message = 'Huddle') => ({id: 'p1', channel_id: 'c1', message, props: {huddle_id: 'h1', ...props}}) as any;
const other = {user_id: 'u2', display_name: 'Zoe', state: 'confirmed' as const};

test('end-for-all only for the starter', () => {
    const h = huddle({participants: [{user_id: 'me', display_name: 'Me', state: 'confirmed'}, other]});
    const {unmount} = render(wrap(makeState([{...h, starter_id: 'me'}]), <HuddleCard post={post()}/>));
    expect(screen.getByRole('button', {name: 'End for everyone'})).toBeInTheDocument();
    unmount();
    render(wrap(makeState([{...h, starter_id: 'u2'}]), <HuddleCard post={post()}/>));
    expect(screen.queryByRole('button', {name: 'End for everyone'})).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Join'})).toBeInTheDocument();
});

test('end calls the server', async () => {
    render(wrap(makeState([huddle()]), <HuddleCard post={post()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'End for everyone'}));
    expect(client.endHuddle).toHaveBeenCalledWith('h1');
});

test('live timer ticks while active and stops on unmount', () => {
    jest.useFakeTimers();
    const now = Date.now();
    jest.setSystemTime(now);
    const {unmount} = render(wrap(makeState([huddle({started_at: now - 65000})]), <HuddleCard post={post()}/>));
    expect(screen.getByText('01:05')).toBeInTheDocument();
    act(() => {
        jest.advanceTimersByTime(3000);
    });
    expect(screen.getByText('01:08')).toBeInTheDocument();
    unmount();
    expect(jest.getTimerCount()).toBe(0);
    jest.useRealTimers();
});

test('ending shows notice, no buttons', () => {
    render(wrap(makeState([huddle({end_requested: true})]), <HuddleCard post={post()}/>));
    expect(screen.getByText('Huddle ending …')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
});

test('empty conference (not end requested): join stays available', () => {
    render(wrap(makeState([huddle({ending: true})]), <HuddleCard post={post()}/>));
    expect(screen.getByRole('button', {name: 'Join'})).toBeEnabled();
    expect(screen.queryByText('Huddle ending …')).not.toBeInTheDocument();
});

test('ended: summary from post, no buttons', () => {
    const p = post({ended: true, attachments: [{text: 'Me, Zoe'}]}, 'Huddle ended · 5 min');
    render(wrap(makeState(), <HuddleCard post={p}/>));
    expect(screen.getByText('Huddle ended · 5 min')).toBeInTheDocument();
    expect(screen.getByText('Me, Zoe')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
});

test('invite picks channel members', async () => {
    const state = makeState([huddle()], {});
    (state.entities.users as any).profiles = {u3: {id: 'u3', username: 'carl'}, me: {id: 'me', username: 'me'}};
    (state.entities.users as any).profilesInChannel = {c1: new Set(['u3', 'me'])};
    render(wrap(state, <HuddleCard post={post()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Invite'}));
    await userEvent.click(screen.getByRole('checkbox', {name: 'carl'}));
    await userEvent.click(screen.getByRole('button', {name: 'Send invitations'}));
    expect(client.inviteToHuddle).toHaveBeenCalledWith('h1', ['u3']);
});

test('invite hides bots, shows empty state, refreshes even loaded members', async () => {
    const state = makeState([huddle()]);
    (state.entities.users as any).profiles = {b1: {id: 'b1', username: 'robot', is_bot: true}};
    (state.entities.users as any).profilesInChannel = {c1: new Set(['b1'])};
    render(wrap(state, <HuddleCard post={post()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Invite'}));
    expect(getProfilesInChannel).toHaveBeenCalledWith('c1', 0, 100); // the cache may hold only a few members
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.getByText('No other members')).toBeInTheDocument();
});

test('invite fetches members not loaded yet', async () => {
    render(wrap(makeState([huddle()]), <HuddleCard post={post()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Invite'}));
    expect(getProfilesInChannel).toHaveBeenCalledWith('c1', 0, 100);
});

test('before the first load the card is neutral, not the ended summary', () => {
    const s = makeState();
    (s as any)['plugins-com.schnaq.meet'].loaded = false;
    render(wrap(s, <HuddleCard post={post()}/>));
    expect(document.querySelector('.Meet-HuddleCard--ended')).toBeNull();
});

test('ended summary renders through host markdown (emoji)', () => {
    window.PostUtils = {
        formatText: (t: string) => t.replace(':headphones:', '<emoji/>'),
        messageHtmlToComponent: (h: string) => <b>{h}</b>,
    };
    try {
        render(wrap(makeState(), <HuddleCard post={post({ended: true}, ':headphones: Huddle ended after 1 min.')}/>));
        expect(screen.getByText('<emoji/> Huddle ended after 1 min.').tagName).toBe('B');
    } finally {
        delete window.PostUtils;
    }
});
