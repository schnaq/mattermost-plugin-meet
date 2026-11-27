import {startRing, stopRing} from './ring_audio';

const created = {oscillators: 0, suspend: 0, instances: 0};

class FakeAudioContext {
    currentTime = 0;
    destination = {};
    constructor() {
        created.instances++;
    }
    resume = () => Promise.resolve();
    suspend = () => {
        created.suspend++;
        return Promise.resolve();
    };
    createGain = () => ({gain: {setValueAtTime: jest.fn(), exponentialRampToValueAtTime: jest.fn()}, connect: (n: unknown) => n});
    createOscillator = () => {
        created.oscillators++;
        return {frequency: {value: 0}, connect: () => ({connect: jest.fn()}), start: jest.fn(), stop: jest.fn()};
    };
}

beforeEach(() => {
    jest.useFakeTimers();
    (window as any).AudioContext = FakeAudioContext;
    Object.assign(created, {oscillators: 0, suspend: 0, instances: 0});
});

afterEach(() => {
    stopRing();
    jest.useRealTimers();
});

test('start plays the two-tone pattern, stop suspends the context', () => {
    startRing();
    expect(created.oscillators).toBe(2);
    jest.advanceTimersByTime(2000);
    expect(created.oscillators).toBe(4);
    stopRing();
    expect(created.suspend).toBe(1);
    jest.advanceTimersByTime(4000);
    expect(created.oscillators).toBe(4);
});

test('starting twice is idempotent', () => {
    startRing();
    startRing();
    expect(created.oscillators).toBe(2);
});
