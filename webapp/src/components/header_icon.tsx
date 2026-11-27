import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import HeadphonesIcon from '@mattermost/compass-icons/components/headphones';
import type {Channel} from '@mattermost/types/channels';

import {Avatars} from './avatars';
import {ActionError, useAction} from './use_action';

import {joinAndOpen, startAndOpen} from '../actions';
import {canJoin, getHuddleForChannel, useMe, useThunkDispatch} from '../state';

import './header_icon.css';

export function HeaderIcon({channel}: {channel: Channel}) {
    const intl = useIntl();
    const dispatch = useThunkDispatch();
    const {run, busy, error} = useAction();
    const huddle = useSelector((s: any) => getHuddleForChannel(s, channel.id));
    const me = useMe();
    const joined = huddle?.participants.some((p) => p.user_id === me);
    const ending = Boolean(huddle) && !canJoin(huddle);

    let label = intl.formatMessage({id: 'meet.header.start', defaultMessage: 'Start huddle'});
    if (ending) {
        label = intl.formatMessage({id: 'meet.card.ending', defaultMessage: 'Huddle ending …'});
    } else if (huddle) {
        label = joined ? intl.formatMessage({id: 'meet.header.in_huddle', defaultMessage: 'In huddle'}) : intl.formatMessage({id: 'meet.header.join', defaultMessage: 'Join huddle'});
    }

    return (
        <div className='Meet-HeaderIcon'>
            <button
                type='button'
                className={`Meet-HeaderIcon__button${huddle ? ' Meet-HeaderIcon__button--active' : ''}`}
                aria-label={label}
                aria-busy={busy}
                disabled={busy || ending}
                title={label}
                onClick={() => run(() => dispatch(huddle ? joinAndOpen(huddle.id) : startAndOpen(channel.id)))}
            >
                <HeadphonesIcon size={18}/>
                {huddle && <Avatars participants={huddle.participants}/>}
                {joined && !ending && <span className='Meet-HeaderIcon__label'>{label}</span>}
            </button>
            <ActionError
                error={error}
                className='Meet-HeaderIcon__error'
            />
        </div>
    );
}
