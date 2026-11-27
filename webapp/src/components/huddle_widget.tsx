import React, {useEffect} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import type {Channel} from '@mattermost/types/channels';

import {getChannel as fetchChannel} from 'mattermost-redux/actions/channels';
import {getChannel, getDirectTeammate} from 'mattermost-redux/selectors/entities/channels';
import {getCurrentTeam, getTeam} from 'mattermost-redux/selectors/entities/teams';
import {isDirectChannel, isGroupChannel} from 'mattermost-redux/utils/channel_utils';

import {Avatars} from './avatars';
import {Elapsed} from './elapsed';
import {HuddleActions} from './huddle_actions';

import {canJoin, getMyHuddle, participantNames, useThunkDispatch} from '../state';

import './meet_card.css';
import './huddle_widget.css';

// Team-relative channel URL as the web app routes it. A DM without a known username falls back
// to its channel name, which the web app redirects.
export function channelPath(team: string, channel: Channel, otherUsername?: string) {
    if (isDirectChannel(channel) && otherUsername) {
        return `/${team}/messages/@${otherUsername}`;
    }
    if (isGroupChannel(channel)) {
        return `/${team}/messages/${channel.name}`;
    }
    return `/${team}/channels/${channel.name}`;
}

// Small bottom-left control while I am in a huddle: back to Meet, invite, jump to the channel.
export function HuddleWidget() {
    const intl = useIntl();
    const dispatch = useThunkDispatch();
    const huddle = useSelector((s: any) => getMyHuddle(s));
    const channel = useSelector((s: any) => (huddle ? getChannel(s, huddle.channel_id) : undefined));

    // DMs/GMs have no team: they open in the current one.
    const team = useSelector((s: any) => (channel?.team_id ? getTeam(s, channel.team_id) : getCurrentTeam(s)));
    const other = useSelector((s: any) => (channel ? getDirectTeammate(s, channel.id) : undefined));
    const channelId = huddle?.channel_id;
    const known = Boolean(channel);

    useEffect(() => {
        if (channelId && !known) {
            dispatch(fetchChannel(channelId));
        }
    }, [channelId, known, dispatch]);

    if (!canJoin(huddle)) {
        return null;
    }
    const name = channel?.display_name || participantNames(huddle.participants);
    const goToChannel = () => {
        if (!channel || !team) {
            return;
        }
        const path = channelPath(team.name, channel, other?.username);
        const history = (window as any).WebappUtils?.browserHistory;
        if (history) {
            history.push(path);
        } else {
            window.location.assign(path);
        }
    };

    return (
        <div
            className='Meet-Card Meet-Card--accent Meet-Widget'
            role='region'
            aria-label={intl.formatMessage({id: 'meet.widget.label', defaultMessage: 'Active huddle'})}
        >
            <div className='Meet-Widget__head'>
                <button
                    type='button'
                    className='Meet-Widget__link'
                    aria-label={intl.formatMessage({id: 'meet.widget.open_channel', defaultMessage: 'Go to {channel}'}, {channel: name})}
                    onClick={goToChannel}
                >
                    {name}
                </button>
                <Elapsed
                    className='Meet-Card__timer'
                    startedAt={huddle.started_at}
                />
            </div>
            <Avatars participants={huddle.participants}/>
            <HuddleActions
                huddle={huddle}
                joinLabel={
                    <FormattedMessage
                        id='meet.widget.switch'
                        defaultMessage='Switch to Meet'
                    />
                }
            />
        </div>
    );
}
