import {render, screen} from '@testing-library/react';

import {SidebarLabel} from './sidebar_label';

import {huddle, makeState, wrap} from '../test_utils';

const channel = {id: 'c1'} as any;

test('shows participant count while active', () => {
    const h = huddle({participants: [{user_id: 'a', display_name: 'A', state: 'confirmed'}, {user_id: 'b', display_name: 'B', state: 'joining'}]});
    render(wrap(makeState([h]), <SidebarLabel channel={channel}/>));
    expect(screen.getByText('2')).toBeInTheDocument();
});

test('renders nothing without huddle', () => {
    const {container} = render(wrap(makeState(), <SidebarLabel channel={channel}/>));
    expect(container).toBeEmptyDOMElement();
});
