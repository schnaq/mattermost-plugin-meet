import {act, render, screen} from '@testing-library/react';
import manifest from 'manifest';

import {RingToast, TOAST_MS} from './ring_toast';

import {ringState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/users', () => ({getUser: jest.fn(() => () => Promise.resolve())}));

const withOutcome = (state: 'declined' | 'missed') => {
    const s: any = ringState({ring: false});
    s[`plugins-${manifest.id}`].outcome = {ring_id: 'r1', state, from: 'me', to: 'caller'};
    return s;
};

test('declined: names the callee, clears the outcome after 5 s', () => {
    jest.useFakeTimers();
    try {
        render(wrap(withOutcome('declined'), <RingToast/>));
        expect(screen.getByRole('status')).toHaveTextContent('anna declined');
        act(() => {
            jest.advanceTimersByTime(TOAST_MS);
        });
        expect(screen.queryByRole('status')).toBeNull();
    } finally {
        jest.useRealTimers();
    }
});

test('missed', () => {
    render(wrap(withOutcome('missed'), <RingToast/>));
    expect(screen.getByRole('status')).toHaveTextContent("anna didn't answer");
});

test('nothing without an outcome', () => {
    const {container} = render(wrap(ringState({ring: false}) as any, <RingToast/>));
    expect(container).toBeEmptyDOMElement();
});
