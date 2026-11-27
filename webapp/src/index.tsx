// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';

import {getCurrentUserLocale} from 'mattermost-redux/selectors/entities/i18n';
import type {DispatchFunc, GetStateFunc} from 'mattermost-redux/types/actions';

import {loadHuddles} from './actions';
import type {HuddleView, RingOutcomeView, RingView} from './client';
import {ChannelBanner} from './components/channel_banner';
import {HeaderIcon} from './components/header_icon';
import {HuddleCard} from './components/huddle_card';
import {HuddleWidget} from './components/huddle_widget';
import {PopoverAction} from './components/popover_action';
import {RingDialog} from './components/ring_dialog';
import {RingToast} from './components/ring_toast';
import {SidebarLabel} from './components/sidebar_label';
import {makeNotificationHook} from './notification_hook';
import {registerSettings} from './settings';
import {huddleChanged, reducer, ringEnded, ringOutcome, ringReceived, stateReset} from './state';
import type {PluginRegistry} from './types/mattermost-webapp';

import de from '../i18n/de.json';
import en from '../i18n/en.json';

type Msg<T> = {data: T};

const translations: Record<string, Record<string, string>> = {en, de};
const pickTranslations = (locale: string) => translations[locale.split(/[-_]/)[0]] ?? en;

export default class Plugin {
    // @see https://developers.mattermost.com/extend/plugins/webapp/reference/
    public initialize(registry: PluginRegistry, store: {dispatch: DispatchFunc; getState: GetStateFunc}) {
        const load = () => store.dispatch(loadHuddles());
        const event = (name: string) => `custom_${manifest.id}_${name}`;

        registry.registerTranslations(pickTranslations);
        registry.registerReducer(reducer);

        // Settings strings are plain text: resolved once, in the locale the user has at startup.
        const dict = pickTranslations(getCurrentUserLocale(store.getState()));
        registerSettings(registry, (id, fallback) => dict[id] ?? fallback);
        registry.registerRootComponent(RingDialog);
        registry.registerRootComponent(RingToast);
        registry.registerGlobalComponent(HuddleWidget);
        registry.registerPopoverUserActionsComponent(PopoverAction);
        registry.registerDesktopNotificationHook(makeNotificationHook(store.getState));
        registry.registerChannelHeaderIcon(HeaderIcon);
        registry.registerSidebarChannelLinkLabelComponent(SidebarLabel);
        registry.registerChannelToastComponent(ChannelBanner);
        registry.registerPostTypeComponent('custom_meet_huddle', HuddleCard);
        registry.registerWebSocketEventHandler(event('huddle_changed'), (msg: Msg<{huddle: string}>) => {
            try {
                store.dispatch(huddleChanged(JSON.parse(msg.data.huddle) as HuddleView));
            } catch {
                // malformed payload, ignore
            }
        });
        registry.registerWebSocketEventHandler(event('ring'), (msg: Msg<RingView>) => {
            store.dispatch(ringReceived(msg.data));
        });
        registry.registerWebSocketEventHandler(event('ring_ended'), (msg: Msg<{ring_id: string}>) => {
            store.dispatch(ringEnded(msg.data.ring_id));
        });
        registry.registerWebSocketEventHandler(event('ring_outcome'), (msg: Msg<RingOutcomeView>) => {
            store.dispatch(ringOutcome(msg.data));
        });
        registry.registerReconnectHandler(() => {
            store.dispatch(stateReset());
            load();
        });
        load();
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
