# Manual testing checklist

Automated tests cover the server logic and the webapp components. Everything that depends on a real
Mattermost client or on Google Meet has to be checked by hand. Run this list before a release and after
every Mattermost server upgrade (the plugin uses a few webapp registry methods marked as internal).

## Setup

- The plugin is installed and configured (see [google-setup.md](google-setup.md)).
- Two or three test users: at least one listed in `GoogleAccounts` and one that is not.
- Two devices or browsers, so one user can be called while another one is in a huddle.

No Google Workspace at hand? The demo build replaces Google with a fake that reports a running conference
and the participants from `MEET_DEMO_PARTICIPANTS`. It covers the Mattermost side (UI, ringing, presence),
not the Google behaviour in the second section. See [CONTRIBUTING.md](../CONTRIBUTING.md#demo-build).

## Experience

- [ ] **Header icon:** a headphones icon sits in the header of every channel and DM. No huddle: a click starts one and opens Meet. Huddle running: the icon is green with avatars and a count, a click joins. When you are in it, the icon shows the "in huddle" state.
- [ ] **Sidebar:** a channel or DM with a running huddle shows headphones and the participant count; the marker disappears after the huddle ends.
- [ ] **Banner:** a banner at the top of the channel shows who is in the huddle; Join works.
- [ ] **Huddle card:** live avatars, running duration, Join and Invite; "End for everyone" only for the person who started the huddle. The card's thread is the huddle chat. After the end the card shows a summary (duration, participants), and recordings, transcripts and notes appear as links in the thread once Google provides them.
- [ ] **Ringing:** starting a huddle in a DM or group DM rings the other members. Invite and `/meet @user` ring the selected users. The dialog shows avatar, channel, ringtone (loops, at most 30 s), Accept, Decline and quick replies. Unanswered calls become "missed huddle call".
- [ ] **Widget:** a floating widget is shown while you are in a huddle: channel, avatars, timer, "Switch to Meet", Invite, jump to the channel.
- [ ] **Presence:** a custom status is set while you are in a huddle and your previous status is restored afterwards. The profile popover offers "Join huddle" or "Start huddle", but not for bots, deactivated users or yourself.
- [ ] **Mobile:** the card is a post attachment with a Join button; ringing arrives as a push notification.
- [ ] **User settings:** with the ring sound off, the dialog appears without sound. With the ring dialog off, no dialog appears.
- [ ] **Language:** the UI and server messages are available in English and German.

## Behaviour that depends on Google

- [ ] **Impersonation of a non-admin:** a regular Workspace user listed in `GoogleAccounts` starts a huddle. The space is created in their name and they are the host.
- [ ] **Signed-in external guest:** an external person with a Google account joins via the link and appears correctly in the participant list.
- [ ] **Anonymous guest:** joining without a Google account. Note the result; it depends on your Workspace settings.
- [ ] **Space restricted after the end:** after "End for everyone", after an abandoned huddle (nobody joins) and after a plugin restart, the space is `RESTRICTED` and the old link no longer lets strangers in.

## Clients and edge cases

- [ ] **Desktop app:** Meet opens in the system browser (no blank window, no blocked popup) on start, join and accept.
- [ ] **Mobile rendering:** the card is readable and the button tappable on iOS and Android.
- [ ] **Do not disturb:** nothing rings (no dialog, no sound); the missed-call hint still makes sense.
- [ ] **Ring on two devices:** both devices of the same user ring; accepting or declining on one stops the other.
- [ ] **Status restore after a config change:** change a plugin setting such as the status text, or disable and enable the plugin, while someone is in a huddle. The user's original custom status comes back and is never overwritten permanently.
