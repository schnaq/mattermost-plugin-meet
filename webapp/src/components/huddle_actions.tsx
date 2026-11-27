import type React from 'react';
import {useState} from 'react';
import {FormattedMessage} from 'react-intl';

import {InvitePanel} from './invite_panel';
import {ActionError, useAction} from './use_action';

import {joinAndOpen} from '../actions';
import * as client from '../client';
import type {HuddleView} from '../client';
import {useMe, useThunkDispatch} from '../state';

import './meet_button.css';
import './meet_card.css';

// The running huddle's actions, shared by card and widget: join (or back to Meet), invite with
// the member picker, and end for everyone (starter only, with showEnd).
export function HuddleActions({huddle, joinLabel, showEnd = false}: {huddle: HuddleView; joinLabel: React.ReactNode; showEnd?: boolean}) {
    const dispatch = useThunkDispatch();
    const me = useMe();
    const {run, busy, error} = useAction();
    const [inviting, setInviting] = useState(false);

    return (
        <>
            <div className='Meet-Card__actions'>
                <button
                    type='button'
                    className='Meet-Button Meet-Button--primary'
                    disabled={busy}
                    aria-busy={busy}
                    onClick={() => run(() => dispatch(joinAndOpen(huddle.id)))}
                >
                    {joinLabel}
                </button>
                <button
                    type='button'
                    className='Meet-Button'
                    aria-expanded={inviting}
                    onClick={() => setInviting((v) => !v)}
                >
                    <FormattedMessage
                        id='meet.card.invite'
                        defaultMessage='Invite'
                    />
                </button>
                {showEnd && huddle.starter_id === me && (
                    <button
                        type='button'
                        className='Meet-Button Meet-Button--danger'
                        disabled={busy}
                        aria-busy={busy}
                        onClick={() => run(() => client.endHuddle(huddle.id))}
                    >
                        <FormattedMessage
                            id='meet.card.end'
                            defaultMessage='End for everyone'
                        />
                    </button>
                )}
            </div>
            {inviting && (
                <InvitePanel
                    huddle={huddle}
                    onDone={() => setInviting(false)}
                />
            )}
            <ActionError error={error}/>
        </>
    );
}
