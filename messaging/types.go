// Package messaging holds the wire types for the messaging service (see spec.md),
// a consumer client that can fan out across every provider, and a permission
// check for providers.
package messaging

import "time"

// ServiceURL identifies the service to the openhost router. It must match the
// manifests exactly: no scheme.
const ServiceURL = "github.com/cloud-in-a-bottle/messaging-service-spec"

// Network slugs.
const (
	NetworkSignal    = "signal"
	NetworkWhatsApp  = "whatsapp"
	NetworkMessenger = "messenger"
	NetworkInstagram = "instagram"
	NetworkRCS       = "rcs"
)

type AccountState string

const (
	StateConnected  AccountState = "connected"
	StateConnecting AccountState = "connecting"
	StateLoggedOut  AccountState = "logged_out"
	StateError      AccountState = "error"
)

type Account struct {
	Network      string       `json:"network"`
	NetworkName  string       `json:"network_name"`
	Self         Person       `json:"self"`
	State        AccountState `json:"state"`
	StateDetail  *string      `json:"state_detail"`
	Syncing      bool         `json:"syncing"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities says what a consumer may do in a chat. A nil pointer or false
// means unsupported.
type Capabilities struct {
	MaxTextLength    int                   `json:"max_text_length,omitempty"`
	Reply            bool                  `json:"reply,omitempty"`
	Edit             *EditCapability       `json:"edit,omitempty"`
	Unsend           *UnsendCapability     `json:"unsend,omitempty"`
	Reactions        *ReactionCapability   `json:"reactions,omitempty"`
	Attachments      *AttachmentCapability `json:"attachments,omitempty"`
	ReadReceipts     bool                  `json:"read_receipts,omitempty"`
	DeliveryReceipts bool                  `json:"delivery_receipts,omitempty"`
	Typing           bool                  `json:"typing,omitempty"`
	LoadOlder        bool                  `json:"load_older,omitempty"`
	StartChat        []string              `json:"start_chat,omitempty"` // "address", "phone"
}

type EditCapability struct {
	MaxAgeS  *int `json:"max_age_s"` // nil = no limit
	MaxCount *int `json:"max_count"` // nil = no limit
}

type UnsendCapability struct {
	MaxAgeS *int `json:"max_age_s"` // nil = no limit
}

type ReactionCapability struct {
	// Adding one past the max replaces your oldest.
	MaxPerPerson int `json:"max_per_person"`
	// nil = any single emoji.
	Emoji []string `json:"emoji"`
}

type AttachmentKind string

const (
	KindImage   AttachmentKind = "image"
	KindVideo   AttachmentKind = "video"
	KindAudio   AttachmentKind = "audio"
	KindVoice   AttachmentKind = "voice"
	KindFile    AttachmentKind = "file"
	KindSticker AttachmentKind = "sticker"
)

type AttachmentCapability struct {
	MaxSizeBytes  int64            `json:"max_size_bytes"`
	Kinds         []AttachmentKind `json:"kinds"`
	Caption       bool             `json:"caption"`
	MaxPerMessage int              `json:"max_per_message"`
}

type Person struct {
	// "" for someone found on the network that the provider has no record of yet.
	ID string `json:"id"`
	// Network identity as a URI, e.g. "signal:<aci>", "tel:+15551234567". Nil for someone not on the network (eg
	// known only by name from an imported export).
	Address   *string `json:"address"`
	Name      *string `json:"name"`
	Phone     *string `json:"phone,omitempty"` // E.164 hint for contact matching
	AvatarURL *string `json:"avatar_url"`
	IsSelf    bool    `json:"is_self"`
}

type ChatKind string

const (
	ChatDM    ChatKind = "dm"
	ChatGroup ChatKind = "group"
)

type Chat struct {
	ID                 string     `json:"id"`
	Kind               ChatKind   `json:"kind"`
	Title              string     `json:"title"`
	AvatarURL          *string    `json:"avatar_url"`
	OtherPerson        *Person    `json:"other_person,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at"`
	LastMessagePreview *string    `json:"last_message_preview"`
	UnreadCount        int        `json:"unread_count"`
	MarkedUnread       bool       `json:"marked_unread"`
	Pinned             bool       `json:"pinned"`
	Archived           bool       `json:"archived"`
	// A timestamp, "forever", or null.
	MutedUntil         *string `json:"muted_until"`
	DisappearingTimerS *int    `json:"disappearing_timer_s"`
	ReadOnly           bool    `json:"read_only"`
}

type Participant struct {
	Person
	Role string `json:"role"` // "admin" | "member"
}

type ChatDetail struct {
	Chat
	Participants []Participant `json:"participants"`
	// nil = the account's capabilities apply.
	Capabilities *Capabilities `json:"capabilities"`
}

type MessageKind string

const (
	KindMessage MessageKind = "message"
	KindEvent   MessageKind = "event"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusSent      Status = "sent"
	StatusDelivered Status = "delivered"
	StatusRead      Status = "read"
	StatusFailed    Status = "failed"
)

type Message struct {
	ID          string       `json:"id"`
	ChatID      string       `json:"chat_id"`
	Sender      Person       `json:"sender"`
	Outgoing    bool         `json:"outgoing"`
	SentAt      time.Time    `json:"sent_at"`
	Kind        MessageKind  `json:"kind"`
	Text        *string      `json:"text"`
	ReplyTo     *ReplyRef    `json:"reply_to"`
	Attachments []Attachment `json:"attachments"`
	Reactions   []Reaction   `json:"reactions"`
	EditedAt    *time.Time   `json:"edited_at"`
	Unsent      bool         `json:"unsent"`
	ExpiresAt   *time.Time   `json:"expires_at"`
	Status      *Status      `json:"status"` // outgoing only
	Error       *string      `json:"error"`
	Event       *Event       `json:"event"` // kind = event only
}

type ReplyRef struct {
	MessageID  *string `json:"message_id"` // nil if the target isn't stored
	SenderName *string `json:"sender_name"`
	Text       *string `json:"text"`
}

type Reaction struct {
	Emoji  string `json:"emoji"`
	Sender Person `json:"sender"`
	Mine   bool   `json:"mine"`
}

// Event is a timeline notice. Type is a convention ("member_joined",
// "timer_changed", "call", ...); consumers render Text.
type Event struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AttachmentStatus string

const (
	AttachmentAvailable AttachmentStatus = "available"
	AttachmentMissing   AttachmentStatus = "missing"
	AttachmentExpired   AttachmentStatus = "expired"
)

type Attachment struct {
	ID          string           `json:"id"`
	Kind        AttachmentKind   `json:"kind"`
	ContentType *string          `json:"content_type"`
	Filename    *string          `json:"filename"`
	Size        *int64           `json:"size"`
	Width       *int             `json:"width"`
	Height      *int             `json:"height"`
	DurationMS  *int             `json:"duration_ms"`
	Caption     *string          `json:"caption"`
	Status      AttachmentStatus `json:"status"`
	URL         string           `json:"url"`
}

// Responses.

type ChatPage struct {
	Chats      []Chat  `json:"chats"`
	NextCursor *string `json:"next_cursor"`
}

// MessagePage is oldest first. A nil cursor means nothing more is stored in
// that direction.
type MessagePage struct {
	Messages     []Message `json:"messages"`
	BeforeCursor *string   `json:"before_cursor"`
	AfterCursor  *string   `json:"after_cursor"`
}

// RecentMessages is newest first, across chats.
type RecentMessages struct {
	Messages   []Message `json:"messages"`
	NextCursor *string   `json:"next_cursor"`
}

type SearchHit struct {
	Message   Message `json:"message"`
	ChatTitle string  `json:"chat_title"`
	Snippet   string  `json:"snippet"` // matches marked as [match]
}

type SearchResults struct {
	Hits []SearchHit `json:"hits"`
}

type People struct {
	People []Person `json:"people"`
}

type SentMessages struct {
	Messages []Message `json:"messages"`
}

// Error is the body of every non-2xx response.
type Error struct {
	Code   string `json:"error"`
	Detail string `json:"detail"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Detail }

const (
	ErrInvalidRequest     = "invalid_request"
	ErrPermissionRequired = "permission_required"
	ErrNotFound           = "not_found"
	ErrNotSupported       = "not_supported"
	ErrRejected           = "rejected"
	ErrNotConnected       = "not_connected"
)

// SSE event names on GET /v1/events.
const (
	EventHello   = "hello"
	EventChat    = "chat"
	EventChats   = "chats"
	EventTyping  = "typing"
	EventAccount = "account"
)
