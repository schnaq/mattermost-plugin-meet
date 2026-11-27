
import {Client4} from 'mattermost-redux/client';

import type {ParticipantView} from '../client';

import './avatars.css';

const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0].toUpperCase()).join('');

// Profile pictures for Mattermost users, initials for guests (no user_id); at most `max` plus "+n".
export function Avatars({participants, max = 3}: {participants: ParticipantView[]; max?: number}) {
    const shown = participants.slice(0, max);
    const extra = participants.length - shown.length;
    return (
        <span className='Meet-Avatars'>
            {shown.map((p, i) => (p.user_id ? (
                <img
                    key={p.user_id}
                    className='Meet-Avatars__avatar'
                    alt={p.display_name}
                    title={p.display_name}
                    src={Client4.getProfilePictureUrl(p.user_id, 0)}
                />
            ) : (
                <span
                    key={`guest-${i}`}
                    className='Meet-Avatars__avatar Meet-Avatars__avatar--initials'
                    role='img'
                    aria-label={p.display_name}
                    title={p.display_name}
                >
                    {initials(p.display_name)}
                </span>
            )))}
            {extra > 0 && <span className='Meet-Avatars__more'>{`+${extra}`}</span>}
        </span>
    );
}
