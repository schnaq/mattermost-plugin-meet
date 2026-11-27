import React, {useEffect, useRef} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {getChannel as fetchChannel} from 'mattermost-redux/actions/channels';
import {getUser as fetchUser} from 'mattermost-redux/actions/users';
import {Client4} from 'mattermost-redux/client';
import {getChannel} from 'mattermost-redux/selectors/entities/channels';
import {getTeammateNameDisplaySetting} from 'mattermost-redux/selectors/entities/preferences';
import {getUser} from 'mattermost-redux/selectors/entities/users';
import {displayUsername} from 'mattermost-redux/utils/user_utils';

import {ActionError, useAction} from './use_action';

import {acceptRing} from '../actions';
import * as client from '../client';
import {startRing, stopRing} from '../ring_audio';
import {getRingPref} from '../settings';
import {getActiveRing, isRingVisible, ringEnded, useThunkDispatch} from '../state';

import './meet_button.css';
import './meet_card.css';
import './ring_dialog.css';

const AUTO_CLOSE_MS = 30000;

// Incoming-call dialog. Escape and the 30 s timeout only close it locally by clearing the ring
// from the store (no decline is sent, the server marks the ring as missed); ring_ended from the
// server clears it too.
export function RingDialog() {
    const intl = useIntl();
    const dispatch = useThunkDispatch();
    const {run, busy, error} = useAction();
    const ring = useSelector((s: any) => getActiveRing(s));
    const visible = useSelector((s: any) => isRingVisible(s));
    const sound = useSelector((s: any) => getRingPref(s, 'ring_sound'));
    const caller = useSelector((s: any) => (ring ? getUser(s, ring.from) : undefined));
    const nameDisplay = useSelector(getTeammateNameDisplaySetting as any) as string;
    const channel = useSelector((s: any) => (ring ? getChannel(s, ring.channel_id) : undefined));
    const acceptRef = useRef<HTMLButtonElement>(null);
    const ringId = visible ? ring?.ring_id : undefined;

    useEffect(() => {
        if (!ring || !ringId) {
            return undefined;
        }
        if (!caller) {
            dispatch(fetchUser(ring.from));
        }
        if (!channel) {
            dispatch(fetchChannel(ring.channel_id));
        }
        const t = setTimeout(() => dispatch(ringEnded(ringId)), AUTO_CLOSE_MS);
        return () => clearTimeout(t);

        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [ringId]);

    useEffect(() => {
        if (!ringId || !sound) {
            return undefined;
        }
        startRing();
        return stopRing;
    }, [ringId, sound]);

    // A floating card, not a modal: it takes focus only if nothing else has it (never from an input).
    useEffect(() => {
        const active = document.activeElement;
        if (ringId && (!active || active === document.body)) {
            acceptRef.current?.focus();
        }
    }, [ringId]);

    if (!ring || !ringId) {
        return null;
    }

    const respond = (action: 'decline' | 'quickreply', text?: string) => run(async () => {
        await client.respondRing(ringId, action, text);
        dispatch(ringEnded(ringId));
    });

    // Escape only applies while focus is inside the dialog.
    const onKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Escape') {
            dispatch(ringEnded(ringId));
        }
    };
    const soon = intl.formatMessage({id: 'meet.ring.reply_soon', defaultMessage: 'On my way'});
    const busyText = intl.formatMessage({id: 'meet.ring.reply_busy', defaultMessage: "Can't right now"});

    return (
        <div
            className='Meet-Card Meet-Ring'
            role='dialog'
            aria-labelledby='meet-ring-title'
            onKeyDown={onKeyDown}
        >
            <img
                className='Meet-Ring__avatar'
                alt=''
                src={Client4.getProfilePictureUrl(ring.from, 0)}
            />
            <div className='Meet-Ring__text'>
                <strong id='meet-ring-title'>
                    <FormattedMessage
                        id='meet.ring.from'
                        defaultMessage='{name} is calling you'
                        values={{name: displayUsername(caller, nameDisplay)}}
                    />
                </strong>
                {channel?.display_name && (
                    <span className='Meet-Ring__channel'>
                        <FormattedMessage
                            id='meet.ring.channel'
                            defaultMessage='in {channel}'
                            values={{channel: channel.display_name}}
                        />
                    </span>
                )}
            </div>
            <div className='Meet-Card__actions'>
                <button
                    ref={acceptRef}
                    type='button'
                    className='Meet-Button Meet-Button--primary Meet-Ring__accept'
                    disabled={busy}
                    aria-busy={busy}
                    onClick={() => run(() => dispatch(acceptRing(ringId)))}
                >
                    <FormattedMessage
                        id='meet.ring.accept'
                        defaultMessage='Accept'
                    />
                </button>
                <button
                    type='button'
                    className='Meet-Button Meet-Button--danger'
                    disabled={busy}
                    onClick={() => respond('decline')}
                >
                    <FormattedMessage
                        id='meet.ring.decline'
                        defaultMessage='Decline'
                    />
                </button>
            </div>
            <div
                className='Meet-Card__actions'
                role='group'
                aria-label={intl.formatMessage({id: 'meet.ring.replies', defaultMessage: 'Quick replies'})}
            >
                {[soon, busyText].map((text) => (
                    <button
                        key={text}
                        type='button'
                        className='Meet-Button'
                        disabled={busy}
                        onClick={() => respond('quickreply', text)}
                    >
                        {text}
                    </button>
                ))}
            </div>
            <ActionError error={error}/>
        </div>
    );
}
