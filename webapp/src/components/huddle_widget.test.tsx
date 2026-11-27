import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import {HuddleWidget, channelPath} from './huddle_widget';

import * as actions from '../actions';
import {huddle, ringState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/users', () => ({getProfilesInChannel: jest.fn(() => () => Promise.resolve())}));
jest.mock('mattermost-redux/actions/channels', () => ({getChannel: jest.fn(() => () => Promise.resolve())}));
jest.mock('../actions', () => ({joinAndOpen: jest.fn(() => () => Promise.resolve())}));

test('renders nothing outside a huddle', () => {
    const {container} = render(wrap(ringState({ring: false}) as any, <HuddleWidget/>));
    expect(container).toBeEmptyDOMElement();
});

test('in a huddle: channel, switch to Meet, invite toggle', async () => {
    render(wrap(ringState({ring: false, huddles: [huddle()]}) as any, <HuddleWidget/>));
    expect(screen.getByRole('button', {name: 'Go to Town Square'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Switch to Meet'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
    await userEvent.click(screen.getByRole('button', {name: 'Invite'}));
    expect(screen.getByRole('group', {name: 'Channel members'})).toBeInTheDocument();
});

test.each([
    ['O', 'town', undefined, '/team/channels/town'],
    ['P', 'secret', undefined, '/team/channels/secret'],
    ['D', 'me__u2', 'ben', '/team/messages/@ben'],
    ['D', 'me__u2', undefined, '/team/channels/me__u2'],
    ['G', 'abc123', undefined, '/team/messages/abc123'],
])('channelPath %s %s', (type, name, other, want) => {
    expect(channelPath('team', {type, name} as any, other)).toBe(want);
});
