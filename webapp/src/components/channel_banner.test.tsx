import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import {ChannelBanner} from './channel_banner';

import * as actions from '../actions';
import {huddle, makeState, wrap} from '../test_utils';

jest.mock('../actions', () => ({joinAndOpen: jest.fn(() => () => Promise.resolve())}));

test('shown to non-participants, joins', async () => {
    render(wrap(makeState([huddle({starter_id: 'x', participants: [{user_id: 'x', display_name: 'X', state: 'confirmed'}]})]), <ChannelBanner/>));
    await userEvent.click(screen.getByRole('button', {name: 'Join'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
});

test('ending: no join offered', () => {
    render(wrap(makeState([huddle({starter_id: 'x', end_requested: true, participants: [{user_id: 'x', display_name: 'X', state: 'confirmed'}]})]), <ChannelBanner/>));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
});

test('hidden for participants', () => {
    const {container} = render(wrap(makeState([huddle()]), <ChannelBanner/>));
    expect(container).toBeEmptyDOMElement();
});

test('hidden without huddle in current channel', () => {
    const {container} = render(wrap(makeState([huddle({channel_id: 'other', participants: []})]), <ChannelBanner/>));
    expect(container).toBeEmptyDOMElement();
});

test('names the first two participants, +n, and the elapsed time', () => {
    jest.useFakeTimers({now: 1_000_000});
    try {
        const people = ['Anna', 'Ben', 'Cleo', 'Dan'].map((n) => ({user_id: n, display_name: n, state: 'confirmed' as const}));
        render(wrap(makeState([huddle({starter_id: 'Anna', started_at: 1_000_000 - 75_000, participants: people})]), <ChannelBanner/>));
        expect(screen.getByText('Huddle with Anna, Ben +2')).toBeInTheDocument();
        expect(screen.getByText('01:15')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: 'Join'})).toBeInTheDocument();
    } finally {
        jest.useRealTimers();
    }
});
