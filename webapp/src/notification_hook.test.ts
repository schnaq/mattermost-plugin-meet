import {makeNotificationHook} from './notification_hook';
import {ringState} from './test_utils';

const args = {title: 't', body: 'b', silent: false, soundName: '', url: '', notify: true};
const card = {type: 'custom_meet_huddle', props: {huddle_id: 'h1'}} as any;
const dm = {type: '', props: {huddle_id: 'h1', attachments: [{actions: [{integration: {context: {huddle_id: 'h1'}}}]}]}} as any;
const other = {type: '', props: {}} as any;
const run = (state: any, post: any) => makeNotificationHook(() => state)(post, {} as any, {} as any, 't', args);

beforeEach(() => {
    jest.spyOn(document, 'hasFocus').mockReturnValue(true);
});
afterEach(() => {
    jest.restoreAllMocks();
});

test('suppresses card and invite DM while the ring dialog is visible', async () => {
    expect((await run(ringState(), card))?.args?.notify).toBe(false);
    expect((await run(ringState(), dm))?.args?.notify).toBe(false);
});

test('passes through when no dialog is visible', async () => {
    expect((await run(ringState({ring: false}), card))?.args?.notify).toBe(true);
    expect((await run(ringState({dialog: 'off'}), card))?.args?.notify).toBe(true);
    expect((await run(ringState({status: 'dnd'}), card))?.args?.notify).toBe(true);
});

test('never touches foreign posts', async () => {
    expect((await run(ringState(), other))?.args?.notify).toBe(true);
});

test('an unrelated plugin card still notifies', async () => {
    const otherCard = {type: 'custom_meet_huddle', props: {huddle_id: 'h2'}} as any;
    const otherDm = {type: '', props: {huddle_id: 'h2'}} as any;
    expect((await run(ringState(), otherCard))?.args?.notify).toBe(true);
    expect((await run(ringState(), otherDm))?.args?.notify).toBe(true);
});

test('passes through while the window is hidden or unfocused', async () => {
    const hidden = jest.spyOn(document, 'hidden', 'get').mockReturnValue(true);
    expect((await run(ringState(), card))?.args?.notify).toBe(true);
    hidden.mockReturnValue(false);
    (document.hasFocus as jest.Mock).mockReturnValue(false);
    expect((await run(ringState(), dm))?.args?.notify).toBe(true);
});

test('only the huddle_id prop counts, not attachment contexts', async () => {
    const legacyDm = {type: '', props: {attachments: [{actions: [{integration: {context: {huddle_id: 'h1'}}}]}]}} as any;
    expect((await run(ringState(), legacyDm))?.args?.notify).toBe(true);
});
