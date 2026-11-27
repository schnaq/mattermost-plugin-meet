import {getRingPref} from './settings';
import {ringState} from './test_utils';

test('ring prefs default to on', () => {
    expect(getRingPref(ringState() as any, 'ring_sound')).toBe(true);
    expect(getRingPref(ringState() as any, 'ring_dialog')).toBe(true);
});

test('off preference is read from the plugin category', () => {
    const s = ringState({sound: 'off', dialog: 'on'}) as any;
    expect(getRingPref(s, 'ring_sound')).toBe(false);
    expect(getRingPref(s, 'ring_dialog')).toBe(true);
});
