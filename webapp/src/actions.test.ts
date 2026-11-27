import {LOAD_RETRY_MS, acceptRing, joinAndOpen, loadHuddles} from './actions';
import * as client from './client';

jest.mock('./client');
const mocked = jest.mocked(client);

describe('open meet', () => {
    const url = 'https://meet.google.com/abc';
    let open: jest.SpyInstance;
    let popup: {location: {href: string}; close: jest.Mock; opener: unknown};
    const ua = navigator.userAgent;

    beforeEach(() => {
        popup = {location: {href: ''}, close: jest.fn(), opener: 'x'};
        open = jest.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    });
    afterEach(() => {
        open.mockRestore();
        delete window.desktop;
        Object.defineProperty(navigator, 'userAgent', {value: ua, configurable: true});
    });

    it('browser: opens blank window before the request, navigates after', async () => {
        let resolve!: (v: {meet_url: string}) => void;
        mocked.joinHuddle.mockReturnValue(new Promise((r) => {
            resolve = r;
        }));
        const p = joinAndOpen('h1')();
        expect(open).toHaveBeenCalledTimes(1);
        expect(open).toHaveBeenCalledWith('about:blank', '_blank');
        expect(popup.location.href).toBe('');
        resolve({meet_url: url});
        await p;
        expect(popup.location.href).toBe(url);
        expect(popup.close).not.toHaveBeenCalled();
    });

    it('browser: error closes the window and rethrows the server text', async () => {
        mocked.joinHuddle.mockRejectedValue(new Error('Huddle beendet'));
        await expect(joinAndOpen('h1')()).rejects.toThrow('Huddle beendet');
        expect(popup.close).toHaveBeenCalled();
    });

    it('desktop app: single window.open after the response', async () => {
        window.desktop = {};
        mocked.joinHuddle.mockResolvedValue({meet_url: url});
        await joinAndOpen('h1')();
        expect(open).toHaveBeenCalledTimes(1);
        expect(open).toHaveBeenCalledWith(url, '_blank', 'noopener');
    });

    it('desktop app detected via user agent; no window on error', async () => {
        Object.defineProperty(navigator, 'userAgent', {value: 'Mozilla Mattermost/6', configurable: true});
        mocked.joinHuddle.mockRejectedValue(new Error('boom'));
        await expect(joinAndOpen('h1')()).rejects.toThrow('boom');
        expect(open).not.toHaveBeenCalled();
    });

    it('acceptRing opens the url and clears the ring', async () => {
        mocked.respondRing.mockResolvedValue({meet_url: url});
        const dispatch = jest.fn();
        await acceptRing('r1')(dispatch);
        expect(popup.location.href).toBe(url);
        expect(dispatch).toHaveBeenCalledWith({type: 'com.schnaq.meet_RING_ENDED', ringId: 'r1'});
    });
});

describe('loadHuddles', () => {
    beforeEach(() => jest.useFakeTimers());
    afterEach(() => jest.useRealTimers());

    it('retries once after 3 s', async () => {
        mocked.fetchHuddles.mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce([]);
        const dispatch = jest.fn();
        const p = loadHuddles()(dispatch);
        await jest.advanceTimersByTimeAsync(LOAD_RETRY_MS - 1);
        expect(mocked.fetchHuddles).toHaveBeenCalledTimes(1);
        expect(dispatch.mock.calls).toEqual([[{type: 'com.schnaq.meet_HUDDLES_LOAD_STARTED'}]]);
        await jest.advanceTimersByTimeAsync(1);
        await p;
        expect(mocked.fetchHuddles).toHaveBeenCalledTimes(2);
        expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({huddles: []}));
    });

    it('a superseded load finishes without its snapshot', async () => {
        let first!: (v: []) => void;
        mocked.fetchHuddles.mockReturnValueOnce(new Promise((r) => {
            first = r;
        })).mockResolvedValueOnce([]);
        const dispatch = jest.fn();
        const p1 = loadHuddles()(dispatch);
        await loadHuddles()(dispatch);
        first([]);
        await p1;
        const loaded = dispatch.mock.calls.map((c) => c[0]).filter((a) => a.type.endsWith('HUDDLES_LOADED'));
        expect(loaded).toEqual([
            expect.objectContaining({huddles: [], stale: false}),
            expect.objectContaining({huddles: undefined, stale: true}),
        ]);
    });

    it('gives up after the retry but marks the state loaded', async () => {
        mocked.fetchHuddles.mockRejectedValue(new Error('down'));
        const dispatch = jest.fn();
        const p = loadHuddles()(dispatch);
        await jest.advanceTimersByTimeAsync(LOAD_RETRY_MS);
        await p;
        expect(mocked.fetchHuddles).toHaveBeenCalledTimes(2);
        expect(dispatch).toHaveBeenCalledTimes(2);
        expect(dispatch.mock.calls[1][0].huddles).toBeUndefined();
    });
});
