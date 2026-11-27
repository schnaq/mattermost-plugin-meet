import React from 'react';
import {FormattedMessage} from 'react-intl';
import {useSelector} from 'react-redux';

import type {Post} from '@mattermost/types/posts';

import {Avatars} from './avatars';
import {Elapsed} from './elapsed';
import {HuddleActions} from './huddle_actions';

import type {HuddleView} from '../client';
import {canJoin, getHuddleForChannel, isLoaded, participantNames} from '../state';

import './meet_card.css';
import './huddle_card.css';

function LiveCard({huddle}: {huddle: HuddleView}) {
    const running = canJoin(huddle);
    return (
        <div className='Meet-Card Meet-Card--accent Meet-HuddleCard'>
            <div className='Meet-HuddleCard__head'>
                <strong>
                    <FormattedMessage
                        id='meet.card.title'
                        defaultMessage='Huddle'
                    />
                </strong>
                {running ? (
                    <Elapsed
                        className='Meet-Card__timer'
                        startedAt={huddle.started_at}
                    />
                ) : (
                    <span>
                        <FormattedMessage
                            id='meet.card.ending'
                            defaultMessage='Huddle ending …'
                        />
                    </span>
                )}
            </div>
            <Avatars
                participants={huddle.participants}
                max={5}
            />
            <span className='Meet-HuddleCard__names'>{participantNames(huddle.participants)}</span>
            {running && (
                <HuddleActions
                    huddle={huddle}
                    joinLabel={
                        <FormattedMessage
                            id='meet.card.join'
                            defaultMessage='Join'
                        />
                    }
                    showEnd={true}
                />
            )}
        </div>
    );
}

declare global {
    interface Window {
        PostUtils?: {
            formatText(text: string, options?: Record<string, unknown>): string;
            messageHtmlToComponent(html: string): React.ReactNode;
        };
    }
}

// Host markdown (emoji like :headphones:, @mentions); plain text where the host API is missing (tests).
const md = (text: string) => {
    const pu = window.PostUtils;
    return pu ? pu.messageHtmlToComponent(pu.formatText(text, {atMentions: true})) : text;
};

// Live from redux while the huddle is active; once ended (or unknown) the post's own summary.
export function HuddleCard({post}: {post: Post}) {
    const huddle = useSelector((s: any) => getHuddleForChannel(s, post.channel_id));
    const loaded = useSelector((s: any) => isLoaded(s));
    const live = !post.props?.ended && huddle?.id === post.props?.huddle_id ? huddle : undefined;
    if (live) {
        return <LiveCard huddle={live}/>;
    }

    // Until the first load the store can't tell active from ended: stay neutral instead of flashing a summary.
    const names = (post.props?.attachments as Array<{text?: string}> | undefined)?.[0]?.text;
    return (
        <div className={`Meet-Card Meet-Card--accent Meet-HuddleCard${loaded || post.props?.ended ? ' Meet-HuddleCard--ended' : ''}`}>
            <span>{md(post.message)}</span>
            {names && <span className='Meet-HuddleCard__names'>{md(names)}</span>}
        </div>
    );
}
