import {useEffect} from 'react';
import {FormattedMessage} from 'react-intl';
import {useSelector} from 'react-redux';

import {getUser as fetchUser} from 'mattermost-redux/actions/users';
import {getTeammateNameDisplaySetting} from 'mattermost-redux/selectors/entities/preferences';
import {getUser} from 'mattermost-redux/selectors/entities/users';
import {displayUsername} from 'mattermost-redux/utils/user_utils';

import {getRingOutcome, ringOutcomeSeen, useThunkDispatch} from '../state';

import './ring_toast.css';

export const TOAST_MS = 5000;

// Tells the caller that a ring was declined or not answered (ring_outcome); clears it after 5 s.
export function RingToast() {
    const dispatch = useThunkDispatch();
    const outcome = useSelector((s: any) => getRingOutcome(s));
    const callee = useSelector((s: any) => (outcome ? getUser(s, outcome.to) : undefined));
    const nameDisplay = useSelector(getTeammateNameDisplaySetting as any) as string;
    const ringId = outcome?.ring_id;
    const to = outcome?.to;
    const known = Boolean(callee);

    useEffect(() => {
        if (to && !known) {
            dispatch(fetchUser(to));
        }
    }, [to, known, dispatch]);
    useEffect(() => {
        if (!ringId) {
            return undefined;
        }
        const t = setTimeout(() => dispatch(ringOutcomeSeen(ringId)), TOAST_MS);
        return () => clearTimeout(t);
    }, [ringId, dispatch]);

    if (!outcome) {
        return null;
    }
    const name = displayUsername(callee, nameDisplay);
    return (
        <div
            className='Meet-Toast'
            role='status'
        >
            {outcome.state === 'declined' ? (
                <FormattedMessage
                    id='meet.toast.declined'
                    defaultMessage='{name} declined'
                    values={{name}}
                />
            ) : (
                <FormattedMessage
                    id='meet.toast.missed'
                    defaultMessage="{name} didn't answer"
                    values={{name}}
                />
            )}
        </div>
    );
}
