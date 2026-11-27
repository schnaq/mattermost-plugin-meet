import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import {HeaderIcon} from './header_icon';
import {ERROR_MS} from './use_action';

import * as actions from '../actions';
import {huddle, makeState, wrap} from '../test_utils';

jest.mock('../actions', () => ({
    startAndOpen: jest.fn(() => () => Promise.resolve()),
    joinAndOpen: jest.fn(() => () => Promise.resolve()),
}));

const channel = {id: 'c1'} as any;
const people = (n: number) => Array.from({length: n}, (_, i) => ({user_id: `u${i}`, display_name: `U${i}`, state: 'confirmed' as const}));

test('no huddle: start button', async () => {
    render(wrap(makeState(), <HeaderIcon channel={channel}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Start huddle'}));
    expect(actions.startAndOpen).toHaveBeenCalledWith('c1');
});

test('active huddle: join with avatar stack max 3 and +n', async () => {
    render(wrap(makeState([huddle({starter_id: 'u0', participants: people(5)})]), <HeaderIcon channel={channel}/>));
    expect(screen.getByText('+2')).toBeInTheDocument();
    expect(screen.getAllByRole('img')).toHaveLength(3);
    await userEvent.click(screen.getByRole('button', {name: 'Join huddle'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
});

test('I am in the huddle: label, click reopens Meet', async () => {
    render(wrap(makeState([huddle()]), <HeaderIcon channel={channel}/>));
    expect(screen.getByText('In huddle')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'In huddle'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
});

test('double click fires one call and disables the button', async () => {
    let resolve: () => void = () => {};
    (actions.startAndOpen as jest.Mock).mockReturnValueOnce(() => new Promise<void>((r) => {
        resolve = r;
    }));
    render(wrap(makeState(), <HeaderIcon channel={channel}/>));
    await userEvent.dblClick(screen.getByRole('button', {name: 'Start huddle'}));
    expect(actions.startAndOpen).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', {name: 'Start huddle'})).toBeDisabled();
    resolve();
    await waitFor(() => expect(screen.getByRole('button', {name: 'Start huddle'})).toBeEnabled());
});

test('ending huddle offers no join', () => {
    render(wrap(makeState([huddle({starter_id: 'u0', end_requested: true, participants: people(1)})]), <HeaderIcon channel={channel}/>));
    expect(screen.getByRole('button', {name: 'Huddle ending …'})).toBeDisabled();
});

test('non-Error rejection shows a generic message', async () => {
    (actions.startAndOpen as jest.Mock).mockReturnValueOnce(() => new Promise((_, reject) => reject('nope')));
    render(wrap(makeState(), <HeaderIcon channel={channel}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Start huddle'}));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong'));
});

test('shows the server error text', async () => {
    (actions.startAndOpen as jest.Mock).mockReturnValueOnce(() => Promise.reject(new Error('Rate limited')));
    render(wrap(makeState(), <HeaderIcon channel={channel}/>));
    await userEvent.click(screen.getByRole('button', {name: 'Start huddle'}));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Rate limited'));
});

test('error clears after 6 s', async () => {
    jest.useFakeTimers();
    try {
        (actions.startAndOpen as jest.Mock).mockReturnValueOnce(() => Promise.reject(new Error('Rate limited')));
        render(wrap(makeState(), <HeaderIcon channel={channel}/>));
        await userEvent.setup({advanceTimers: jest.advanceTimersByTime}).click(screen.getByRole('button', {name: 'Start huddle'}));
        await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
        act(() => {
            jest.advanceTimersByTime(ERROR_MS);
        });
        expect(screen.queryByRole('alert')).toBeNull();
    } finally {
        jest.useRealTimers();
    }
});
