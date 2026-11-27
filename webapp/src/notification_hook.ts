import type {Post} from '@mattermost/types/posts';

import {getActiveRing, isRingVisible} from './state';
import type {DesktopNotificationArgs} from './types/mattermost-webapp';

// Silences the desktop notification for the ringing huddle's own posts (card, invite DM) while
// the ring dialog is up and the app is visible and focused (otherwise nobody sees the dialog);
// other plugin posts still notify.
export const makeNotificationHook = (getState: () => Record<string, any>) => async (post: Post, _p: unknown, _c: unknown, _t: string, args: DesktopNotificationArgs) => {
    if (document.hidden || !document.hasFocus()) {
        return {args};
    }
    const state = getState();
    const id = post.props?.huddle_id; // set on the card and the invite DM
    if (id && id === getActiveRing(state)?.huddle_id && isRingVisible(state)) {
        return {args: {...args, notify: false}};
    }
    return {args};
};
