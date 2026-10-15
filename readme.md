# messaging-service-spec

A Cloud in a Bottle service that lets apps read and send messages through any
messaging app (Signal, WhatsApp, Messenger, Instagram, later RCS) with one
interface. Same shape as
[health-data-service-spec](https://github.com/cloud-in-a-bottle/health-data-service-spec):
each source app *provides* the service and stays the source of truth, and
consumers query it live.

- [`spec.md`](spec.md) — the API: data model, endpoints, events, permissions.
- [`messaging/`](messaging) — Go module (`github.com/cloud-in-a-bottle/messaging-service-spec/messaging`):
  wire types, a consumer client with provider fan-out, and `RequireGrants`
  middleware for providers.

Status: **draft 0.1**, not implemented by any provider yet.

## Design notes

**One provider = one account.** Each messaging app runs one login, so the
openhost provider is the natural unit; there's no account ID in the API. A
unified inbox lists providers and tags everything with the provider `app_id`.
Two WhatsApp accounts would be two installed apps.

**Chats are never merged.** A chat lives in exactly one provider. Linking the
same person across networks is a separate concern (see Identity below), and
what it produces is a hint for display, not a merged thread.

**Scope is "a chat is a person or group with one timeline".** Quote-replies are
in; Slack/Discord channels and threads are out and would want their own spec.
mautrix's `thread` capability is deliberately not exposed.

**Capabilities, not network checks.** Consumers decide what to offer (react,
edit, unsend, attach, typing) from `capabilities`, never from `network`. Account-
level capabilities are defaults and a chat may override them, because the
networks really do vary per chat (WhatsApp announcement groups, Messenger
community chats). For the bridge-core apps, this is a projection of mautrix's
`event.RoomFeatures`, which is already stored in `room_state`.

**IDs are opaque, provider-local strings.** bridge-core and Signal both key
everything by local sqlite integers, and network message IDs aren't always
available (bridge-core's `remote_id` is null until the network confirms; Signal
identifies a message by author + timestamp). Opaque strings let providers use
whatever is stable for them. The one identifier meant to travel outside a
provider is a person's **address**.

**Live updates are invalidations.** Both existing SSE streams already work this
way: "chat X changed, refetch". It keeps one source of truth, tolerates dropped
events, and is cheap to implement through the router.

**Pagination uses `(sent_at, id)` cursors.** bridge-core and Signal both page by
`before=<sent_at>`, which can skip messages that share a timestamp across a page
boundary. The spec's cursors are opaque, so providers can fix this without
changing the API.

**Writes are fire-and-follow.** A send returns once stored, possibly `pending`,
and status updates arrive as `chat` events. That's how bridge-core already works;
Signal (synchronous sends) just returns `sent`. `client_id` makes sends safe to
retry through the router.

**Permissions are `read` and `write`,** both global strings. Providers must
enforce them; message history is about as sensitive as data gets.

## Identity across networks (later step)

The address URI (`signal:<aci>`, `whatsapp:<id>`, `tel:+1…`) is the hook for
linking people. The [contacts](https://github.com/cloud-in-a-bottle/contacts)
app stores vCards, and vCard already has a field for exactly this:

```
IMPP;X-SERVICE-TYPE=WhatsApp:whatsapp:15551234567
IMPP;X-SERVICE-TYPE=Signal:signal:6f1c...
```

contacts already preserves `IMPP` lines it doesn't edit, and they sync to phones
over CardDAV. Later, a small contacts service (look up by address, list
addresses for a contact, add an address) would let the inbox show contact
names and photos and offer "message on WhatsApp / Signal" from a person,
without merging any chats.

## Plan

1. **Agree this spec.**
2. **bridge-core provider.** A `service` package serving `/messaging/v1/...`
   from the existing store behind `messaging.RequireGrants`, mapping
   `room_features` to capabilities. The three apps then each add a
   `[[services.v2.provides]]` block. Along the way: fix the `sent_at`-only
   cursor, and have the app's own UI read capabilities instead of hard-coding
   one reaction per person and unlimited edit/unsend.
3. **Inbox meta-app (Go).** Uses the client here for fan-out and serves its
   own UI. The message-rendering code in bridge-core's `web/static/app.js`
   (`renderMessage`, attachments, reactions, menus) gets pulled into a module
   that takes an API base and capabilities, so both the per-network apps and
   the inbox use it. The browser talks to the inbox's Go server, which proxies
   to providers, so the app token stays server-side.
4. **Signal.** Free once it's on bridge-core; otherwise its own provider.
5. **Contacts service** for identity linking.
6. **RCS.** open-rcs-client is a standalone Android app today, so it needs a
   server-side component that provides this service. The spec already allows
   for it: `tel:` addresses, no edits, delivery/read receipts via IMDN.
