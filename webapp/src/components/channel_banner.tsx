import {FormattedMessage} from 'react-intl';
import {useSelector} from 'react-redux';

import HeadphonesIcon from '@mattermost/compass-icons/components/headphones';

import {getCurrentChannelId} from 'mattermost-redux/selectors/entities/common';

import {Elapsed} from './elapsed';
import {ActionError, useAction} from './use_action';

import {joinAndOpen} from '../actions';
import {canJoin, getHuddleForChannel, participantNames, useMe, useThunkDispatch} from '../state';

import './channel_banner.css';

// The channel toast slot passes no channel, so the current one comes from redux.
export function ChannelBanner() {
    const dispatch = useThunkDispatch();
    const {run, busy, error} = useAction();
    const me = useMe();
    const huddle = useSelector((s: any) => getHuddleForChannel(s, getCurrentChannelId(s)));
    if (!huddle || huddle.participants.some((p) => p.user_id === me)) {
        return null;
    }
    const names = participantNames(huddle.participants, 2);
    return (
        <div className='Meet-ChannelBanner'>
            <HeadphonesIcon size={16}/>
            <span className='Meet-ChannelBanner__text'>
                {names ? (
                    <FormattedMessage
                        id='meet.banner.with'
                        defaultMessage='Huddle with {names}'
                        values={{names}}
                    />
                ) : (
                    <FormattedMessage
                        id='meet.banner.running'
                        defaultMessage='A huddle is running in this channel'
                    />
                )}
            </span>
            <Elapsed
                className='Meet-ChannelBanner__timer'
                startedAt={huddle.started_at}
            />
            {canJoin(huddle) ? (
                <button
                    type='button'
                    className='Meet-ChannelBanner__join'
                    disabled={busy}
                    aria-busy={busy}
                    onClick={() => run(() => dispatch(joinAndOpen(huddle.id)))}
                >
                    <FormattedMessage
                        id='meet.banner.join'
                        defaultMessage='Join'
                    />
                </button>
            ) : (
                <FormattedMessage
                    id='meet.card.ending'
                    defaultMessage='Huddle ending …'
                />
            )}
            <ActionError error={error}/>
        </div>
    );
}
