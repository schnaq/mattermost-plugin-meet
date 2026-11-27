import type {HuddleView} from './client';
import {canJoin, getActiveRing, getHuddleForChannel, getHuddleOfUser, getMyHuddle, getRingOutcome, huddleChanged, huddlesLoadStarted, huddlesLoaded, participantNames, reducer, ringEnded, ringOutcome, ringOutcomeSeen, ringReceived, stateReset} from './state';

const h = (id: string, channel: string, users: string[], extra: Partial<HuddleView> = {}): HuddleView => ({
    id,
    channel_id: channel,
    starter_id: users[0],
    started_at: 1,
    ending: false,
    end_requested: false,
    participants: users.map((u) => ({user_id: u, display_name: u, state: 'confirmed' as const})),
    ...extra,
});
const root = (s: ReturnType<typeof reducer>) => ({'plugins-com.schnaq.meet': s, entities: {users: {currentUserId: 'me'}}});

describe('reducer', () => {
    it('loads, changes, removes ended, resets', () => {
        let s = reducer(undefined, huddlesLoaded([h('a', 'c1', ['x'])]));
        expect(Object.keys(s.huddles)).toEqual(['a']);
        s = reducer(s, huddleChanged(h('a', 'c1', ['x', 'y'])));
        expect(s.huddles.a.participants).toHaveLength(2);
        s = reducer(s, huddleChanged(h('a', 'c1', ['x'], {ended: true})));
        expect(s.huddles).toEqual({});
        s = reducer(s, huddlesLoaded([h('b', 'c2', ['x'])]));
        expect(reducer(s, stateReset()).huddles).toEqual({});
        expect(s.loaded).toBe(true);
        expect(reducer(s, stateReset()).loaded).toBe(false);
    });

    it('tracks the ring and only clears the matching one', () => {
        const ring = {ring_id: 'r1', huddle_id: 'a', channel_id: 'c1', from: 'u'};
        let s = reducer(undefined, ringReceived(ring));
        expect(getActiveRing(root(s))).toEqual(ring);
        s = reducer(s, ringEnded('other'));
        expect(s.ring).toEqual(ring);
        s = reducer(s, ringEnded('r1'));
        expect(s.ring).toBeUndefined();
    });
});

describe('load race', () => {
    it('events received during the load win over the older snapshot', () => {
        let s = reducer(undefined, huddlesLoadStarted());
        s = reducer(s, huddleChanged(h('a', 'c1', ['x'])));
        s = reducer(s, huddleChanged(h('a', 'c1', ['x'], {ended: true})));
        s = reducer(s, huddleChanged(h('b', 'c2', ['y', 'z'])));
        s = reducer(s, huddlesLoaded([h('a', 'c1', ['x']), h('b', 'c2', ['y']), h('c', 'c3', ['w'])]));
        expect(Object.keys(s.huddles).sort()).toEqual(['b', 'c']);
        expect(s.huddles.b.participants).toHaveLength(2);
        expect(s.recent).toBeUndefined();

        // Once no load is in flight, a snapshot applies as is.
        s = reducer(s, huddlesLoadStarted());
        s = reducer(s, huddlesLoaded([]));
        expect(s.huddles).toEqual({});
    });

    it('a reconnect during a load: the old load neither drops new events nor wins', () => {
        for (const order of [['old', 'new'], ['new', 'old']]) {
            let s = reducer(undefined, huddlesLoadStarted());
            s = reducer(s, huddleChanged(h('y', 'c9', ['q'])));
            s = reducer(s, stateReset());
            s = reducer(s, huddlesLoadStarted());
            s = reducer(s, huddleChanged(h('x', 'c1', ['x'], {ended: true})));
            for (const which of order) {
                s = reducer(s, which === 'old' ? huddlesLoaded(undefined, true) : huddlesLoaded([h('x', 'c1', ['x'])]));
            }
            expect(s.huddles).toEqual({});
            expect(s.loaded).toBe(true);
            expect(s.loads).toBe(0);
        }
    });

    it('overlapping loads keep the events until the last one finished', () => {
        let s = reducer(undefined, huddlesLoadStarted());
        s = reducer(s, huddlesLoadStarted());
        s = reducer(s, huddleChanged(h('a', 'c1', ['x'])));
        s = reducer(s, huddlesLoaded([]));
        expect(Object.keys(s.huddles)).toEqual(['a']);
        s = reducer(s, huddlesLoaded([]));
        expect(Object.keys(s.huddles)).toEqual(['a']);
        expect(s.loads).toBe(0);
        expect(reducer(s, stateReset()).recent).toBeUndefined();
    });
});

describe('load failure and ring outcome', () => {
    it('a failed load marks loaded but keeps websocket huddles', () => {
        let s = reducer(undefined, huddleChanged(h('a', 'c1', ['x'])));
        s = reducer(s, huddlesLoaded());
        expect(s.loaded).toBe(true);
        expect(Object.keys(s.huddles)).toEqual(['a']);
    });

    it('stores the ring outcome; seen clears only the matching one', () => {
        const outcome = {ring_id: 'r1', state: 'declined' as const, to: 'anna'};
        let s = reducer(undefined, ringOutcome(outcome));
        expect(getRingOutcome(root(s))).toEqual(outcome);
        s = reducer(s, ringOutcomeSeen('other'));
        expect(s.outcome).toEqual(outcome);
        s = reducer(s, ringOutcome({...outcome, ring_id: 'r2', state: 'missed'}));
        s = reducer(s, ringOutcomeSeen('r1'));
        expect(s.outcome?.state).toBe('missed');
        expect(reducer(s, ringOutcomeSeen('r2')).outcome).toBeUndefined();
    });
});

describe('selectors', () => {
    const s = reducer(undefined, huddlesLoaded([h('a', 'c1', ['me', 'x']), h('b', 'c2', ['y'])]));
    it('finds by channel, user and current user', () => {
        expect(getHuddleForChannel(root(s), 'c2')?.id).toBe('b');
        expect(getHuddleOfUser(root(s), 'y')?.id).toBe('b');
        expect(getHuddleOfUser(root(s), 'nobody')).toBeUndefined();
        expect(getMyHuddle(root(s))?.id).toBe('a');
    });
});

describe('helpers', () => {
    it('canJoin: only a running huddle', () => {
        expect(canJoin(undefined)).toBe(false);
        expect(canJoin(h('a', 'c1', ['x']))).toBe(true);
        expect(canJoin(h('a', 'c1', ['x'], {end_requested: true}))).toBe(false);
        expect(canJoin(h('a', 'c1', ['x'], {ending: true}))).toBe(true); // empty conference: still joinable
    });

    it('participantNames: all, or max with +n', () => {
        const ps = h('a', 'c1', ['Anna', 'Ben', 'Cleo', 'Dan']).participants;
        expect(participantNames(ps)).toBe('Anna, Ben, Cleo, Dan');
        expect(participantNames(ps, 2)).toBe('Anna, Ben +2');
        expect(participantNames(ps.slice(0, 2), 2)).toBe('Anna, Ben');
        expect(participantNames([])).toBe('');
    });
});
