import React from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import HeadphonesIcon from '@mattermost/compass-icons/components/headphones';
import type {Channel} from '@mattermost/types/channels';

import {getHuddleForChannel} from '../state';

import './sidebar_label.css';

export function SidebarLabel({channel}: {channel: Channel}) {
    const intl = useIntl();
    const huddle = useSelector((s: any) => getHuddleForChannel(s, channel.id));
    if (!huddle) {
        return null;
    }
    const count = huddle.participants.length;
    const label = intl.formatMessage({id: 'meet.sidebar.active', defaultMessage: 'Huddle in progress, {count, plural, one {# participant} other {# participants}}'}, {count});
    return (
        <span
            className='Meet-SidebarLabel'
            role='img'
            aria-label={label}
            title={label}
        >
            <HeadphonesIcon size={12}/>
            {count}
        </span>
    );
}
