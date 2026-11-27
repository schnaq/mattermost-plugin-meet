import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import {HuddleActions} from './huddle_actions';

import * as actions from '../actions';
import * as client from '../client';
import {huddle, makeState, wrap} from '../test_utils';

jest.mock('mattermost-redux/actions/users', () => ({getProfilesInChannel: jest.fn(() => () => Promise.resolve())}));
jest.mock('../client');
jest.mock('../actions', () => ({joinAndOpen: jest.fn(() => () => Promise.resolve())}));

test('join, invite toggle, end for the starter, errors inline', async () => {
    (client.endHuddle as jest.Mock).mockRejectedValueOnce(new Error('nope'));
    render(wrap(makeState([huddle()]), (
        <HuddleActions
            huddle={huddle()}
            joinLabel='Hop in'
            showEnd={true}
        />
    )));
    await userEvent.click(screen.getByRole('button', {name: 'Hop in'}));
    expect(actions.joinAndOpen).toHaveBeenCalledWith('h1');
    const invite = screen.getByRole('button', {name: 'Invite'});
    await userEvent.click(invite);
    expect(invite).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('group', {name: 'Channel members'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'End for everyone'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('nope');
});

test('no end button without showEnd or for others', () => {
    const {unmount} = render(wrap(makeState(), (
        <HuddleActions
            huddle={huddle()}
            joinLabel='Join'
        />
    )));
    expect(screen.queryByRole('button', {name: 'End for everyone'})).toBeNull();
    unmount();
    render(wrap(makeState(), (
        <HuddleActions
            huddle={huddle({starter_id: 'u2'})}
            joinLabel='Join'
            showEnd={true}
        />
    )));
    expect(screen.queryByRole('button', {name: 'End for everyone'})).toBeNull();
});
