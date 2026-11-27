import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import {RingDialog} from './ring_dialog';

import * as actions from '../actions';
import * as client from '../client';
import * as audio from '../ring_audio';
import {ringState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/users', () => ({getUser: jest.fn(() => () => Promise.resolve())}));
jest.mock('mattermost-redux/actions/channels', () => ({getChannel: jest.fn(() => () => Promise.resolve())}));
jest.mock('../actions', () => ({acceptRing: jest.fn(() => () => Promise.resolve())}));
jest.mock('../ring_audio', () => ({startRing: jest.fn(), stopRing: jest.fn()}));
jest.mock('../client', () => ({respondRing: jest.fn(() => Promise.resolve({}))}));

const show = (o: Parameters<typeof ringState>[0] = {}) => render(wrap(ringState(o) as any, <RingDialog/>));

test('shows caller, channel and plays the sound', () => {
    show();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText(/anna/)).toBeInTheDocument();
    expect(screen.getByText(/Town Square/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Accept'})).toHaveFocus();
    expect(audio.startRing).toHaveBeenCalled();
});

test.each([['dnd'], ['ooo']])('no dialog and no sound when status is %s', (status) => {
    show({status});
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(audio.startRing).not.toHaveBeenCalled();
});

test('no dialog when ring_dialog is off', () => {
    show({dialog: 'off'});
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(audio.startRing).not.toHaveBeenCalled();
});

test('sound only when ring_sound is on', () => {
    show({sound: 'off'});
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(audio.startRing).not.toHaveBeenCalled();
});

test('accept calls acceptRing', async () => {
    show();
    await userEvent.click(screen.getByRole('button', {name: 'Accept'}));
    expect(actions.acceptRing).toHaveBeenCalledWith('r1');
});

test('decline responds and closes the dialog', async () => {
    show();
    await userEvent.click(screen.getByRole('button', {name: 'Decline'}));
    expect(client.respondRing).toHaveBeenCalledWith('r1', 'decline', undefined);
    expect(screen.queryByRole('dialog')).toBeNull();
});

test('a quick reply responds with its text', async () => {
    show();
    await userEvent.click(screen.getByRole('button', {name: "Can't right now"}));
    expect(client.respondRing).toHaveBeenCalledWith('r1', 'quickreply', "Can't right now");
});

test('accept error is shown inline', async () => {
    (actions.acceptRing as jest.Mock).mockReturnValueOnce(() => Promise.reject(new Error('boom')));
    show();
    await userEvent.click(screen.getByRole('button', {name: 'Accept'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('boom');
});

test('auto-closes after 30 s and stops the sound', () => {
    jest.useFakeTimers();
    try {
        show();
        act(() => {
            jest.advanceTimersByTime(29000);
        });
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        act(() => {
            jest.advanceTimersByTime(1500);
        });
        expect(screen.queryByRole('dialog')).toBeNull();
        expect(audio.stopRing).toHaveBeenCalled();
    } finally {
        jest.useRealTimers();
    }
});

test('Escape closes locally without declining', async () => {
    show();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(client.respondRing).not.toHaveBeenCalled();
});

test('Escape outside the dialog does not close it', async () => {
    show();
    (document.activeElement as HTMLElement).blur();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
});

test('does not steal focus from an input; Enter there does not accept', async () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    try {
        input.focus();
        show();
        expect(screen.getByRole('dialog')).not.toHaveAttribute('aria-modal');
        expect(input).toHaveFocus();
        await userEvent.keyboard('hi{Enter}');
        expect(input).toHaveValue('hi');
        expect(actions.acceptRing).not.toHaveBeenCalled();
        expect(screen.getByRole('dialog')).toBeInTheDocument();
    } finally {
        input.remove();
    }
});
