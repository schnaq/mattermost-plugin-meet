import React, {useEffect, useMemo, useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {getProfilesInChannel} from 'mattermost-redux/actions/users';
import {getTeammateNameDisplaySetting} from 'mattermost-redux/selectors/entities/preferences';
import {makeGetProfilesInChannel} from 'mattermost-redux/selectors/entities/users';
import {displayUsername} from 'mattermost-redux/utils/user_utils';

import {ActionError, useAction} from './use_action';

import * as client from '../client';
import type {HuddleView} from '../client';
import {useMe, useThunkDispatch} from '../state';

import './meet_button.css';
import './invite_panel.css';

// Channel-member picker shared by the huddle card and the huddle widget.
export function InvitePanel({huddle, onDone}: {huddle: HuddleView; onDone: () => void}) {
    const intl = useIntl();
    const dispatch = useThunkDispatch();
    const {run, busy, error} = useAction();
    const me = useMe();
    const getProfiles = useMemo(() => makeGetProfilesInChannel(), []);
    const members = useSelector((s: any) => getProfiles(s, huddle.channel_id));
    const nameDisplay = useSelector(getTeammateNameDisplaySetting as any) as string;

    // Cached members show at once; refreshed on every open, since the cache may hold only a few.
    // ponytail: first 100 members only; page or search if channels get larger.
    useEffect(() => {
        dispatch(getProfilesInChannel(huddle.channel_id, 0, 100));
    }, [dispatch, huddle.channel_id]);
    const [picked, setPicked] = useState<string[]>([]);
    const present = new Set(huddle.participants.map((p) => p.user_id));
    const candidates = members.filter((u) => u.id !== me && !u.is_bot && !present.has(u.id));
    const toggle = (id: string) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));

    return (
        <div
            className='Meet-InvitePanel'
            role='group'
            aria-label={intl.formatMessage({id: 'meet.card.invite_group', defaultMessage: 'Channel members'})}
        >
            {candidates.length === 0 && (
                <FormattedMessage
                    id='meet.card.invite_empty'
                    defaultMessage='No other members'
                />
            )}
            {candidates.map((u) => (
                <label
                    key={u.id}
                    className='Meet-InvitePanel__member'
                >
                    <input
                        type='checkbox'
                        checked={picked.includes(u.id)}
                        onChange={() => toggle(u.id)}
                    />
                    {displayUsername(u, nameDisplay)}
                </label>
            ))}
            <button
                type='button'
                className='Meet-Button Meet-Button--primary'
                disabled={busy || picked.length === 0}
                aria-busy={busy}
                onClick={() => run(async () => {
                    await client.inviteToHuddle(huddle.id, picked);
                    onDone();
                })}
            >
                <FormattedMessage
                    id='meet.card.invite_send'
                    defaultMessage='Send invitations'
                />
            </button>
            <ActionError error={error}/>
        </div>
    );
}
