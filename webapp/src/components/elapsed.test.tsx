import {act, render, screen} from '@testing-library/react';
import React from 'react';

import {Elapsed} from './elapsed';

test('renders mm:ss, ticks every second, stops on unmount', () => {
    jest.useFakeTimers({now: 1_000_000});
    try {
        const {unmount} = render(<Elapsed startedAt={1_000_000 - 65_000}/>);
        expect(screen.getByText('01:05')).toBeInTheDocument();
        act(() => {
            jest.advanceTimersByTime(3000);
        });
        expect(screen.getByText('01:08')).toBeInTheDocument();
        unmount();
        expect(jest.getTimerCount()).toBe(0);
    } finally {
        jest.useRealTimers();
    }
});

test('a start in the future shows 00:00', () => {
    render(<Elapsed startedAt={Date.now() + 5000}/>);
    expect(screen.getByText('00:00')).toBeInTheDocument();
});
