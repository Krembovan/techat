package model

type MessageType string

const (
	MsgRegister    MessageType = "register"
	MsgGlobal      MessageType = "global"
	MsgDirect      MessageType = "direct"
	MsgPresence    MessageType = "presence"
	MsgTyping      MessageType = "typing"
	MsgRead        MessageType = "read"
	MsgFileOffer   MessageType = "file_offer"
	MsgFileChunk   MessageType = "file_chunk"
	MsgKeyExchange MessageType = "key_exchange"
	MsgError       MessageType = "error"
)

type Message struct {
	Type       MessageType `json:"type"`
	Sender     string      `json:"sender,omitempty"`
	Recipient  string      `json:"to,omitempty"`
	Content    string      `json:"content,omitempty"`
	PubKey     string      `json:"pubkey,omitempty"`
	Nonce      string      `json:"nonce,omitempty"`
	Timestamp  int64       `json:"timestamp"`

	FileName   string `json:"filename,omitempty"`
	FileSize   int64  `json:"filesize,omitempty"`
	FileID     string `json:"file_id,omitempty"`
	ChunkData  string `json:"chunk_data,omitempty"`
	ChunkSeq   int    `json:"chunk_seq,omitempty"`
	ChunkTotal int    `json:"chunk_total,omitempty"`

	Status     string `json:"status,omitempty"`
}

type StoredMessage struct {
	ID          int64
	Sender      string
	Recipient   string
	Content     string
	Nonce       string
	IsEncrypted bool
	Timestamp   string
	FilePath    string
}

type User struct {
	Username string
	PubKey   string
	Online   bool
}

type Chat struct {
	ID       string
	Name     string
	IsGlobal bool
	Unread   int
}
