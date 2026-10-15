# Messaging service — v0.1 (draft)

Service URL: `github.com/cloud-in-a-bottle/messaging-service-spec`

A provider is one messaging app (Signal, WhatsApp, Messenger, Instagram, later RCS)
holding **one account**. The provider is the source of truth: it stores the
history and talks to the network. Consumers query it live through the openhost
router; they don't sync or copy data unless they choose to.

Scope: networks where **a chat is a person or a group with one primary
timeline**, with optional quote-replies. Slack/Discord-style servers, channels
and threads are out of scope and would need a different service.

All paths below are relative to the provider's `endpoint`, e.g.
`[[services.v2.provides]] endpoint = "/messaging/"`. Consumers call
`$BOTTLE_ROUTER_URL/api/services/v2/call/<shortname>/v1/...` with
`Authorization: Bearer $BOTTLE_APP_TOKEN`.

## Conventions

- JSON in and out, except attachment bytes and multipart sends.
- Timestamps are ISO 8601 strings in UTC with millisecond precision
  (`2026-10-15T14:03:22.120Z`), the same as the health service.
- **IDs are opaque strings.** They are stable for the lifetime of the
  provider's data and unique *within that provider*. Consumers address
  something globally as `(provider app_id, id)` and must not parse IDs.
- **Cursors are opaque strings.** Never build one; pass back what you got.
- Unknown fields must be ignored by consumers. New optional fields and new
  capability keys are minor-version changes.
- Errors: `{"error": "<code>", "detail": "<human text>"}`.

| HTTP | `error` | meaning |
|---|---|---|
| 400 | `invalid_request` | bad parameters |
| 403 | `permission_required` | missing grant (openhost shape, see Permissions) |
| 404 | `not_found` | no such chat/message/attachment |
| 409 | `not_supported` | the network or this chat doesn't allow it (see capabilities) |
| 422 | `rejected` | the network refused it (edit window passed, etc.) |
| 503 | `not_connected` | the account is logged out or offline |

## Data model

### Addresses

A participant's network identity is an **address**: a URI, `<scheme>:<id>`.
It's the stable handle other apps (contacts in particular) use to refer to
someone on a network.

| network | scheme | id | example |
|---|---|---|---|
| Signal | `signal` | ACI (UUID) | `signal:6f1c…-…` |
| WhatsApp | `whatsapp` | JID user | `whatsapp:15551234567` (or the `@lid` id) |
| Messenger | `messenger` | Facebook user ID | `messenger:100004…` |
| Instagram | `instagram` | Instagram user ID | `instagram:1784…` |
| RCS / SMS | `tel` | E.164 | `tel:+15551234567` |

Only people have addresses; chats are addressed by their `id`. Someone a
provider knows only by name (eg from an imported export) has a null address.

Where the network tells us a phone number (Signal, WhatsApp), the person also
gets a `phone` (E.164) as a hint for contact matching. The address is the
identity; the phone number is not.

### Account

```json
{
  "network": "whatsapp",
  "network_name": "WhatsApp",
  "self": {Person},
  "state": "connected",            // connected | connecting | logged_out | error
  "state_detail": null,
  "syncing": false,                // initial history still arriving
  "capabilities": {Capabilities}   // defaults for this account's chats
}
```

`network` is a lowercase slug: `signal`, `whatsapp`, `messenger`, `instagram`, `rcs`.

Logging in and setup happen in the provider's own UI, at
`https://<app_name>.<zone domain>/`; this API has no login calls.

### Capabilities

What a consumer can do. Account-level capabilities are the defaults; a chat may
override them (WhatsApp announcement groups disallow replies, Messenger
community chats disallow typing, Instagram drops captions). A missing key or
`false` means unsupported.

```json
{
  "max_text_length": 65536,
  "reply": true,
  "edit":   {"max_age_s": 900, "max_count": null},   // null = no limit; key absent = no edits
  "unsend": {"max_age_s": 172800},                   // delete-for-everyone, own messages only
  "reactions": {
    "max_per_person": 1,          // adding one past the max replaces your oldest
    "emoji": null                 // null = any single emoji; else the allowed list
  },
  "attachments": {
    "max_size_bytes": 2147483648,
    "kinds": ["image", "video", "audio", "voice", "file", "sticker"],
    "caption": true,
    "max_per_message": 1          // more files are sent as separate messages
  },
  "read_receipts": true,          // marking read tells the other side, and outgoing messages reach "read"
  "delivery_receipts": true,      // outgoing messages reach status "delivered"
  "typing": true,
  "load_older": true,             // POST /v1/chats/{id}/history can fetch older messages from the network
  "start_chat": ["address", "phone"]   // what POST /v1/chats accepts
}
```

`attachments.kinds` lists what can be *sent* through this API; any kind may be
received.

For the mautrix-based providers this is a projection of `com.beeper.room_features`
(`event.RoomFeatures`), which bridge-core already stores in `room_state`.

### Person

```json
{
  "id": "p_42",                    // "" for someone found on the network the provider has no record of yet
  "address": "whatsapp:15551234567",  // null for someone not on the network
  "name": "Ada Lovelace",          // best name the network gives; may be null
  "phone": "+15551234567",         // optional hint
  "avatar_url": "v1/avatars/ab12…",// relative to the service base; null if none
  "is_self": false
}
```

### Chat

```json
{
  "id": "c_17",
  "kind": "dm",                    // dm | group
  "title": "Ada Lovelace",         // display name; for a DM, the other person's name
  "avatar_url": "v1/avatars/…",
  "other_person": {Person},        // dm only
  "last_message_at": "…",
  "last_message_preview": "see you then",
  "unread_count": 2,
  "marked_unread": false,
  "pinned": false,
  "archived": false,
  "muted_until": null,             // timestamp, "forever", or null
  "disappearing_timer_s": null,
  "read_only": false               // e.g. imported history with no live chat
}
```

`GET /v1/chats/{id}` adds:

```json
{
  "participants": [{Person, "role": "admin" | "member"}],
  "capabilities": {Capabilities} | null   // null = account defaults apply
}
```

### Message

```json
{
  "id": "m_9001",
  "chat_id": "c_17",
  "sender": {Person},              // self for outgoing
  "outgoing": true,
  "sent_at": "…",                  // the sender's timestamp; the ordering key
  "kind": "message",               // message | event
  "text": "see you then",          // plain text; null when unsent or media-only
  "reply_to": {                    // null if not a reply
    "message_id": "m_8990",        // null if the target isn't stored
    "sender_name": "Ada Lovelace",
    "text": "dinner at 7?"
  },
  "attachments": [{Attachment}],
  "reactions": [{"emoji": "👍", "sender": {Person}, "mine": false}],
  "edited_at": null,
  "unsent": false,                 // tombstone: text and attachments removed
  "expires_at": null,
  "status": "delivered",           // outgoing only: pending | sent | delivered | read | failed
  "error": null,                   // set when status = failed
  "event": null                    // kind = event only, see below
}
```

`kind: "event"` covers timeline notices — someone joined, the disappearing timer
changed, a call. `event` is `{"type": "<slug>", "text": "<human text>"}`. Consumers
render `text`; `type` is a convention, not an enum (`member_joined`,
`member_left`, `title_changed`, `timer_changed`, `call`, …).

Messages with one sender timestamp sort by `(sent_at, id)`; providers must give
a stable total order within a chat.

### Attachment

```json
{
  "id": "a_77",
  "kind": "image",                 // image | video | audio | voice | file | sticker
  "content_type": "image/jpeg",
  "filename": "IMG_2231.jpg",
  "size": 183223,
  "width": 1600, "height": 1200,
  "duration_ms": null,
  "caption": null,
  "status": "available",           // available | missing | expired
  "url": "v1/attachments/a_77"
}
```

## Reading

| | | |
|---|---|---|
| `GET /v1/account` | → Account | |
| `GET /v1/chats` | → `{chats, next_cursor}` | ordered by `last_message_at` desc. `?limit=` (default 50, max 500), `?cursor=`, `?archived=include\|exclude\|only` (default exclude) |
| `GET /v1/chats/{id}` | → Chat + participants + capabilities | |
| `GET /v1/chats/{id}/messages` | → `{messages, before_cursor, after_cursor, has_more_before, has_more_after}` | `messages` oldest first. With no cursor, the newest page. `?before=` / `?after=` page from a cursor; `?around=<message_id>` centres a page on a message (for search hits and reply jumps). `?limit=` default 50, max 500. The cursors are the positions of the first and last message (null only for an empty page), so `after=<after_cursor>` later fetches what's arrived since. |
| `GET /v1/messages/{id}` | → Message | |
| `GET /v1/messages` | → `{messages, next_cursor}` | across all chats, newest first. `?since=` / `?until=` timestamps, `?limit=`, `?cursor=`. For "what came in today". |
| `GET /v1/search` | → `{hits: [{message, chat_title, snippet}]}` | `?q=` (word prefixes), optional `?chat_id=`, `?limit=` (max 100). `snippet` marks matches as `[match]`. |
| `GET /v1/people` | → `{people}` | `?q=` searches people the account knows (and the network's directory, if it has one), for starting chats |
| `GET /v1/attachments/{id}` | → bytes | `Content-Type` set; `Cache-Control: immutable` |
| `GET /v1/avatars/{id}` | → bytes | |
| `GET /v1/events` | SSE | see below |

Pages with a cursor never skip or repeat a message, even if many share a
timestamp: cursors are `(sent_at, id)` positions, not bare timestamps.

### Live updates — `GET /v1/events`

Server-sent events. Payloads are **invalidations**: they say what changed, and the
consumer re-reads. That keeps one source of truth and lets events be dropped
safely.

| event | data | meaning |
|---|---|---|
| `hello` | `{}` | (re)connected — anything may have changed, refetch what's on screen |
| `chat` | `{"chat_id"}` | the chat, its messages, or its summary changed |
| `chats` | `{}` | the chat list changed in a way not tied to one chat |
| `typing` | `{"chat_id", "person_id", "typing": bool}` | |
| `account` | `{}` | account state or capabilities changed |

A comment line (`: keepalive`) at least every 15 s.

## Writing

All writes need the `write` grant. The account must be `connected` (else 503).

| | body | → |
|---|---|---|
| `POST /v1/chats` | `{"address": "…"}` or `{"phone": "+1…"}` or `{"person_id": "…"}` | Chat (existing DM if there is one) |
| `POST /v1/chats/{id}/messages` | multipart: `text`, `reply_to` (message id), `files[]`, `client_id` | `202` → `{"messages": [Message]}` |
| `PATCH /v1/messages/{id}` | `{"text": "…"}` | `204` (edit) |
| `DELETE /v1/messages/{id}` | | `204` (unsend for everyone; a `failed` message is just discarded) |
| `POST /v1/messages/{id}/retry` | | `202` → Message (failed only) |
| `POST /v1/messages/{id}/reactions` | `{"emoji": "👍"}` | `204` |
| `DELETE /v1/messages/{id}/reactions/{emoji}` | | `204` (removes your reaction) |
| `POST /v1/chats/{id}/read` | `{"up_to": "<message id>"}` (optional; default latest) | `204` |
| `POST /v1/chats/{id}/typing` | `{"typing": true}` | `204` |
| `POST /v1/chats/{id}/history` | | `202`: fetch a page of older messages from the network (`load_older`); a `chat` event follows |

Sending:
- A send returns as soon as the provider has stored it. Messages come back as
  `pending` if the network hasn't confirmed yet; providers whose sends are
  synchronous may return `sent` straight away. Consumers follow the status via
  `chat` events.
- One request can become several messages (one per file where
  `max_per_message` is 1, with the text as the last file's caption), so the
  response is a list.
- `client_id` (any string, unique per send) makes the request idempotent: a
  retried POST with the same `client_id` returns the original messages instead
  of sending twice.
- Actions a capability rules out return 409 `not_supported`; ones the network
  refuses (an edit past the window) return 422 `rejected`.

## Permissions

Providers **must** check `X-OpenHost-Permissions` on every request. Grants are
plain strings, scope `global`:

| grant | allows |
|---|---|
| `read` | every `GET` |
| `write` | sending, editing, unsending, reacting, marking read, typing, starting chats |

A missing grant returns the openhost 403:

```json
{"error": "permission_required", "required_grant": {"grant": "read", "scope": "global"}}
```

Consumer manifest:

```toml
[[services.v2.consumes]]
service = "github.com/cloud-in-a-bottle/messaging-service-spec"
shortname = "messaging"
version = ">=0.1.0"
grants = ["read", "write"]
```

Provider manifest:

```toml
[[services.v2.provides]]
service = "github.com/cloud-in-a-bottle/messaging-service-spec"
version = "0.1.0"
endpoint = "/messaging/"
```

## Many providers

Every messaging app provides this service, so a consumer that wants everything
lists providers (`GET $BOTTLE_ROUTER_URL/api/services/v2/providers?service=<url>`,
which returns an array of `{app_id, app_name, status, ...}`) and calls each
running one with `X-OpenHost-Provider: <app_id>`. Because chats are ordered by
`last_message_at`, a unified chat list is a merge of each provider's first page;
fetch more from whichever provider's page runs out first.
