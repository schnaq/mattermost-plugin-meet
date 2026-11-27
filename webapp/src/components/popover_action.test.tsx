import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import {PopoverAction} from './popover_action';

import * as actions from '../actions';
import {huddle, ringState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/channels', () => ({createDirectChannel: jest.fn(() => () => Promise.resolve({data: {id: 'dm1'}}))}));
jest.mock('../actions', () => ({
    startAndOpen: jest.fn(() => () => Promise.resolve()),
    joinAndOpen: jest.fn(() => () => Promise.resolve()),
}));

const user = (over = {}) => ({id: 'u2', is_bot: false, delete_at: 0, ...over}) as any;
const theirs = huddle({participants: [{user_id: 'u2', display_name: 'Zoe', state: 'confirmed'}]});

test('not for myself, bots or deactivated users', () => {
    const s = ringState({ring: false}) as any;
    expect(render(wrap(s, <PopoverAction user={user({id: 'me'})}/>)).container).toBeEmptyDOMElement();
    expect(render(wrap(s, <PopoverAction user={user({is_bot: true})}/>)).container).toBeEmptyDOMElement();
    expect(render(wrap(s, <PopoverAction user={user({delete_at: 5})}/>)).container).toBeEmptyDOMElement();
});

test('target in a huddle: join', async () => {
    render(wrap(ringState({ring: false, huddles: [theirs]}) as any, <PopoverAction user={user()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Join huddle'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
});

test('otherwise: start in the DM channel', async () => {
    render(wrap(ringState({ring: false}) as any, <PopoverAction user={user()}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Start huddle'}));
    const arg = (actions.startAndOpen as jest.Mock).mock.calls[0][0];
    await expect(arg).resolves.toBe('dm1');
});

test('a DM promise nobody awaits does not reject unhandled', async () => {
    const {createDirectChannel} = jest.requireMock('mattermost-redux/actions/channels');
    createDirectChannel.mockImplementationOnce(() => () => Promise.resolve({error: {message: 'no dm'}}));
    const unhandled = jest.fn();
    process.on('unhandledRejection', unhandled);
    try {
        render(wrap(ringState({ring: false}) as any, <PopoverAction user={user()}/>));
        await userEvent.click(screen.getByRole('button', {name: 'Start huddle'})); // startAndOpen mock ignores the promise
        await new Promise((r) => setTimeout(r, 0));
        expect(unhandled).not.toHaveBeenCalled();
    } finally {
        process.off('unhandledRejection', unhandled);
    }
});
