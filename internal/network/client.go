package network

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"techat/internal/model"
)

var ErrSendBufferFull = errors.New("send buffer full")

type Client struct {
	addr      string
	username  string
	pubKeyB64 string
	conn      *websocket.Conn
	sendCh    chan []byte
	recvCh    chan model.Message
	done      chan struct{}
	mu        sync.Mutex
	Connected bool
}

func NewClient(addr, username string, pubKey []byte) *Client {
	return &Client{
		addr:      addr,
		username:  username,
		pubKeyB64: base64.StdEncoding.EncodeToString(pubKey),
		sendCh:    make(chan []byte, 256),
		recvCh:    make(chan model.Message, 512),
		done:      make(chan struct{}),
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.addr+"/ws", nil)
	if err != nil {
		return err
	}
	c.conn = conn
	c.Connected = true

	go c.readPump()
	go c.writePump()

	regMsg := model.Message{
		Type:   model.MsgRegister,
		Sender: c.username,
		PubKey: c.pubKeyB64,
	}
	return c.sendJSON(regMsg)
}

func (c *Client) sendJSON(msg model.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case c.sendCh <- data:
		return nil
	default:
		return ErrSendBufferFull
	}
}

func (c *Client) readPump() {
	defer func() {
		c.mu.Lock()
		c.Connected = false
		c.mu.Unlock()
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg model.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		select {
		case c.recvCh <- msg:
		default:
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case data, ok := <-c.sendCh:
			if !ok {
				return
			}
			c.mu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
					c.mu.Unlock()
					return
				}
			}
			c.mu.Unlock()
		case <-ticker.C:
			c.mu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					c.mu.Unlock()
					return
				}
			}
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}

func (c *Client) Send(msg model.Message) error {
	return c.sendJSON(msg)
}

func (c *Client) SendGlobal(content string) error {
	return c.sendJSON(model.Message{
		Type:    model.MsgGlobal,
		Content: content,
	})
}

func (c *Client) SendDirect(recipient, content, nonce string) error {
	return c.sendJSON(model.Message{
		Type:      model.MsgDirect,
		Recipient: recipient,
		Content:   content,
		Nonce:     nonce,
	})
}

func (c *Client) SendTyping(recipient string) error {
	return c.sendJSON(model.Message{
		Type:      model.MsgTyping,
		Recipient: recipient,
	})
}

func (c *Client) Addr() string {
	return c.addr
}

func (c *Client) SendReadReceipt(recipient string) error {
	return c.sendJSON(model.Message{
		Type:      model.MsgRead,
		Recipient: recipient,
	})
}

func (c *Client) SendFileOffer(recipient, filename string, filesize int64, fileID string) error {
	return c.sendJSON(model.Message{
		Type:      model.MsgFileOffer,
		Recipient: recipient,
		FileName:  filename,
		FileSize:  filesize,
		FileID:    fileID,
	})
}

func (c *Client) SendFileChunk(recipient, fileID string, seq, total int, chunkData string) error {
	return c.sendJSON(model.Message{
		Type:       model.MsgFileChunk,
		Recipient:  recipient,
		FileID:     fileID,
		ChunkSeq:   seq,
		ChunkTotal: total,
		ChunkData:  chunkData,
	})
}

func (c *Client) Messages() <-chan model.Message {
	return c.recvCh
}

func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
