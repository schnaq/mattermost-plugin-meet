# Google setup

The plugin talks to Google as a **service account** that is allowed, through **domain-wide delegation**, to act
as users of your Workspace domain. You need:

- a Google Cloud project (any project in your organization),
- permission to create a service account and a key in it,
- Google Workspace super admin access to authorize the delegation.

## 1. Create a Google Cloud project

Open the [Google Cloud console](https://console.cloud.google.com/) and create a project, or reuse an existing
one, for example `mattermost-meet`.

## 2. Enable the APIs

In **APIs & Services > Library** enable:

- **Google Meet REST API**
- **Admin SDK API**

## 3. Create the service account and key

1. **IAM & Admin > Service Accounts > Create service account**, for example `mattermost-meet`. No project roles are needed.
2. Open the account, note its **Unique ID** (the numeric client ID), then **Keys > Add key > Create new key > JSON** and download the file.

If key creation is blocked, your organization enforces the policy `iam.disableServiceAccountKeyCreation`;
an organization administrator has to allow it for this project.

## 4. Authorize domain-wide delegation

In the [Admin console](https://admin.google.com/) go to **Security > Access and data control > API controls >
Manage Domain Wide Delegation > Add new**.

- **Client ID:** the service account's unique ID from step 3.
- **OAuth scopes** (exactly these two, comma-separated):

  ```
  https://www.googleapis.com/auth/meetings.space.created,https://www.googleapis.com/auth/admin.directory.user.readonly
  ```

Changes can take a few minutes to apply.

## 5. Configure the plugin

In **System Console > Plugins > Meet Huddles**:

1. Paste the **whole content of the JSON key file** into *Service account JSON key*.
2. Add the people who should host huddles as themselves to *Google accounts*, one per line:

   ```
   alice=alice@example.com
   bob=bob@example.com
   ```

3. Optionally set a *Fallback owner email*, a Workspace account that hosts huddles of everyone else (for
   example guests or users not listed above). Without it, unlisted users can join but not start huddles.
4. Save and enable the plugin. Start a huddle from a channel to test; a configuration error is shown in the
   Mattermost UI and the server log.

## Security notes

- **The key is a secret.** Anyone with the JSON key and the delegation can act as users of your domain within the two scopes. The plugin stores it in the Mattermost configuration (database, backups), where system admins can read it. Do not commit it, do not share it. Keep the original in a secrets manager and rotate it regularly: create a new key, paste it into the plugin, delete the old key in Google Cloud.
- **Least privilege.** The scopes are deliberately narrow:
  - `meetings.space.created` lets the plugin create Meet spaces and read, end and list artifacts of **spaces it created itself**. It cannot see your other meetings. Do not replace it with `meetings.space.readonly`, which would expose the Meet history of the whole domain.
  - `admin.directory.user.readonly` is used only to resolve a participant's Google user ID to an email address (basic profile data, no mailbox or Drive content).
- **Use a dedicated service account and project**, and keep project roles empty.
- **Choose the fallback owner carefully.** Huddles started by unlisted users are hosted by that account, and recordings or transcripts (if enabled in Meet) belong to it.
- **Mattermost emails are not trusted.** Impersonation only ever uses addresses you enter in *Google accounts* or *Fallback owner email*.
