import {FormattedMessage} from 'react-intl';
import {useSelector} from 'react-redux';

import type {UserProfile} from '@mattermost/types/users';

import {createDirectChannel} from 'mattermost-redux/actions/channels';

import {ActionError, useAction} from './use_action';

import {joinAndOpen, startAndOpen} from '../actions';
import {getHuddleOfUser, useMe, useThunkDispatch} from '../state';

import './meet_button.css';
import './popover_action.css';

// Profile popover action: join the huddle the user is in, else start one in our DM.
export function PopoverAction({user, hide}: {user: UserProfile; hide?: () => void}) {
    const dispatch = useThunkDispatch();
    const {run, busy, error} = useAction();
    const me = useMe();
    const huddle = useSelector((s: any) => getHuddleOfUser(s, user.id));
    if (user.id === me || user.is_bot || user.delete_at > 0) {
        return null;
    }

    const click = () => {
        if (huddle) {
            run(async () => {
                await dispatch(joinAndOpen(huddle.id));
                hide?.();
            });
            return;
        }

        // DM creation is idempotent on the server; the promise keeps the click gesture for the popup.
        const dmId = dispatch(createDirectChannel(me, user.id)).then((r) => {
            if (!r.data) {
                throw new Error(r.error?.message);
            }
            return r.data.id;
        });
        dmId.catch(() => undefined); // awaited inside run; this only stops an unhandled rejection if run skips it
        run(async () => {
            await dispatch(startAndOpen(dmId));
            hide?.();
        });
    };

    return (
        <div className='Meet-PopoverAction'>
            <button
                type='button'
                className='Meet-Button Meet-PopoverAction__button'
                disabled={busy}
                aria-busy={busy}
                onClick={click}
            >
                {huddle ? (
                    <FormattedMessage
                        id='meet.popover.join'
                        defaultMessage='Join huddle'
                    />
                ) : (
                    <FormattedMessage
                        id='meet.popover.start'
                        defaultMessage='Start huddle'
                    />
                )}
            </button>
            <ActionError error={error}/>
        </div>
    );
}
