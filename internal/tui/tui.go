package tui

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Krembovan/techat/internal/crypto"
	"github.com/Krembovan/techat/internal/db"
	"github.com/Krembovan/techat/internal/model"
	"github.com/Krembovan/techat/internal/network"
)

const (
	sidebarWidth  = 22
	inputHeight   = 3
	statusHeight  = 1
	chunkSize     = 64 * 1024
)

type ChatItem struct {
	ID       string
	Name     string
	IsGlobal bool
	Online   bool
	Unread   int
}

type FileTransfer struct {
	FileID      string
	FileName    string
	FileSize    int64
	Sender      string
	Chunks      [][]byte
	TotalChunks int
	Received    int
	InProgress  bool
	OutputPath  string
}

type Model struct {
	viewport   viewport.Model
	input      textarea.Model
	spinner    spinner.Model
	help       help.Model

	username   string
	activeChat string
	chatList   []ChatItem
	selChatIdx int
	messages   []string

	users      map[string]*model.User
	typing     map[string]time.Time

	client     *network.Client
	msgCh      <-chan model.Message
	connected  bool

	width      int
	height     int
	ready      bool
	focusInput bool

	keyPair    *crypto.KeyPair
	sharedKeys map[string][]byte

	db         *db.Database
	keyPath    string
	dataDir    string

	fileXfers  map[string]*FileTransfer

	err        error
	showHelp   bool
	panicArmed bool
	panicTime  time.Time
}

func New(username string, client *network.Client, database *db.Database,
	keyPair *crypto.KeyPair, keyPath, dataDir string) Model {

	ta := textarea.New()
	ta.Placeholder = "Type a message... (Alt+Enter=newline)"
	ta.Focus()
	ta.CharLimit = 10000
	ta.SetWidth(80)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(Green)
	s.Spinner = spinner.Dot

	vp := viewport.New(0, 0)
	vp.YPosition = 0

	chatList := []ChatItem{
		{ID: "global", Name: "🌐 Global", IsGlobal: true, Online: true},
	}

	return Model{
		input:      ti,
		spinner:    s,
		viewport:   vp,
		help:       help.New(),
		username:   username,
		activeChat: "global",
		chatList:   chatList,
		selChatIdx: 0,
		users:      make(map[string]*model.User),
		typing:     make(map[string]time.Time),
		client:     client,
		msgCh:      client.Messages(),
		connected:  true,
		focusInput: true,
		keyPair:    keyPair,
		sharedKeys: make(map[string][]byte),
		db:         database,
		keyPath:    keyPath,
		dataDir:    dataDir,
		fileXfers:  make(map[string]*FileTransfer),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForMsg(m.msgCh))
}

func waitForMsg(ch <-chan model.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return disconnectedMsg{}
		}
		return netMessage{Msg: msg}
	}
}

type netMessage struct {
	Msg model.Message
}

type disconnectedMsg struct{}

type reconnectSuccess struct {
	client *network.Client
	msgCh  <-chan model.Message
}
type reconnectFailed struct{}
type panicDone struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.ready = true
		}
		m.updateLayout()

	case tea.KeyMsg:
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit

		case msg.String() == "ctrl+p":
			if m.panicArmed && time.Since(m.panicTime) < 3*time.Second {
				return m, m.panic()
			}
			m.panicArmed = true
			m.panicTime = time.Now()
			m.addSystemMsg("!!! PANIC ARMED: Press Ctrl+P again within 3s to wipe all data !!!")
			return m, nil
		default:
			m.panicArmed = false

		case msg.String() == "tab":
			m.focusInput = !m.focusInput
			if m.focusInput {
				m.input.Focus()
			} else {
				m.input.Blur()
			}
			return m, nil

		case msg.String() == "enter" && m.focusInput && !msg.Alt:
			return m.handleSend()

		case msg.String() == "enter" && !m.focusInput:
			if m.selChatIdx >= 0 && m.selChatIdx < len(m.chatList) {
				m.switchChat(m.chatList[m.selChatIdx].ID)
			}
			m.focusInput = true
			m.input.Focus()
			return m, nil

		case msg.String() == "up" && !m.focusInput:
			if m.selChatIdx > 0 {
				m.selChatIdx--
			}
			return m, nil

		case msg.String() == "down" && !m.focusInput:
			if m.selChatIdx < len(m.chatList)-1 {
				m.selChatIdx++
			}
			return m, nil

		case msg.String() == "?":
			m.showHelp = !m.showHelp
			return m, nil

		case msg.String() == "ctrl+r":
			m.client.Close()
			return m, m.reconnect()

		default:
			if m.focusInput {
				if msg.Alt && msg.String() == "enter" {
					m.input.InsertString("\n")
					return m, nil
				}
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)

				if m.activeChat != "global" && m.connected {
					go m.client.SendTyping(m.activeChat)
				}
			}
		}

	case netMessage:
		cmds = append(cmds, m.handleNetMsg(msg.Msg))

	case disconnectedMsg:
		m.connected = false
		m.addSystemMsg("⚠ Disconnected from relay")
		return m, nil

	case reconnectSuccess:
		m.client = msg.client
		m.msgCh = msg.msgCh
		m.connected = true

	case reconnectFailed:
		m.addSystemMsg("✖ Reconnection failed")

	case panicDone:
		return m, tea.Quit
	}

	if m.connected {
		cmds = append(cmds, waitForMsg(m.msgCh))
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) updateLayout() {
	vpWidth := m.width - sidebarWidth - 2
	if vpWidth < 10 {
		vpWidth = 10
	}
	vpHeight := m.height - inputHeight - statusHeight - 1

	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight
	m.input.SetWidth(vpWidth - 2)
}

func (m *Model) addSystemMsg(text string) {
	m.messages = append(m.messages, SystemMsgStyle.Render(text))
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) addErrorMsg(text string) {
	m.messages = append(m.messages, ErrorMsgStyle.Render(text))
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) addMessage(sender, body, timestamp string, isSelf bool) {
	ts := MessageTimeStyle.Render(timestamp)
	var senderLabel string
	if isSelf {
		senderLabel = SelfSenderStyle.Render("you")
	} else {
		senderLabel = SenderStyle.Render(sender)
	}
	rendered := renderMessage(body)
	line := fmt.Sprintf("%s %s %s", ts, senderLabel, rendered)
	m.messages = append(m.messages, line)
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) addFileMsg(sender, filename string, filesize int64) {
	ts := MessageTimeStyle.Render(time.Now().Format("15:04:05"))
	sizeStr := formatFileSize(filesize)
	senderLabel := SenderStyle.Render(sender)
	line := fmt.Sprintf("%s %s 📎 %s (%s)", ts, senderLabel,
		FileMsgStyle.Render(filename), FileProgressStyle.Render(sizeStr))
	m.messages = append(m.messages, line)
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) addFileProgress(fileID string, received, total int) {
	ts := MessageTimeStyle.Render(time.Now().Format("15:04:05"))
	pct := 0
	if total > 0 {
		pct = received * 100 / total
	}
	bar := progressBar(pct, 20)
	line := fmt.Sprintf("%s %s [%s] %d/%d", ts,
		FileProgressStyle.Render("⬆ File transfer:"),
		bar, received, total)
	for i, msg := range m.messages {
		if strings.Contains(msg, fileID) || strings.Contains(msg, "File transfer:") {
			m.messages[i] = line
		}
	}
	// If no existing progress line, append
	found := false
	for _, msg := range m.messages {
		if strings.Contains(msg, "File transfer:") && strings.Contains(msg, fileID) {
			found = true
			break
		}
	}
	if !found {
		m.messages = append(m.messages, line)
	}
	m.viewport.SetContent(strings.Join(m.messages, "\n"))
	m.viewport.GotoBottom()
}

func renderMessage(text string) string {
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") {
		code := strings.TrimPrefix(text, "```")
		code = strings.TrimSuffix(code, "```")
		code = strings.TrimSpace(code)
		return CodeBlockStyle.Render(code)
	}

	words := strings.Fields(text)
	var parts []string
	for _, word := range words {
		if strings.HasPrefix(word, "**") && strings.HasSuffix(word, "**") {
			inner := strings.TrimPrefix(word, "**")
			inner = strings.TrimSuffix(inner, "**")
			parts = append(parts, BoldStyle.Render(inner))
		} else if strings.HasPrefix(word, "*") && strings.HasSuffix(word, "*") {
			inner := strings.TrimPrefix(word, "*")
			inner = strings.TrimSuffix(inner, "*")
			parts = append(parts, ItalicStyle.Render(inner))
		} else if strings.HasPrefix(word, "`") && strings.HasSuffix(word, "`") {
			inner := strings.TrimPrefix(word, "`")
			inner = strings.TrimSuffix(inner, "`")
			parts = append(parts, CodeBlockStyle.Render(inner))
		} else {
			parts = append(parts, MessageBodyStyle.Render(word))
		}
	}
	return strings.Join(parts, " ")
}

func progressBar(pct, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return lipgloss.NewStyle().Foreground(Green).Render(bar)
}

func (m *Model) switchChat(chatID string) {
	m.activeChat = chatID
	m.messages = nil
	m.viewport.SetContent("")

	if chatID == "global" {
		m.addSystemMsg("--- Global Chat ---")
		m.addSystemMsg("Messages are not encrypted and not saved")
	} else {
		m.loadPrivateChat(chatID)
	}

	for i := range m.chatList {
		if m.chatList[i].ID == chatID {
			m.chatList[i].Unread = 0
			break
		}
	}
}

func (m *Model) loadPrivateChat(contact string) {
	m.addSystemMsg(fmt.Sprintf("--- Private chat with %s (E2E Encrypted) ---", contact))

	stored, err := m.db.GetMessages(contact, 100, 0)
	if err != nil {
		return
	}

	pubKey, hasKey := m.loadPubKey(contact)
	sharedKey := m.getSharedKey(pubKey)

	for _, sm := range stored {
		if sm.IsEncrypted && sharedKey != nil {
			decrypted, err := crypto.DecryptMessage(sharedKey, sm.Content, sm.Nonce)
			if err != nil {
				continue
			}
			isSelf := sm.Sender == m.username
			m.addMessage(sm.Sender, decrypted, sm.Timestamp, isSelf)
		} else if !sm.IsEncrypted && sm.Content != "" {
			isSelf := sm.Sender == m.username
			m.addMessage(sm.Sender, sm.Content, sm.Timestamp, isSelf)
		}
	}
}

func (m *Model) loadPubKey(username string) (string, bool) {
	if user, ok := m.users[username]; ok && user.PubKey != "" {
		return user.PubKey, true
	}
	pubKey, err := m.db.GetPubKey(username)
	return pubKey, err == nil
}

func (m *Model) getSharedKey(pubKeyB64 string) []byte {
	if pubKeyB64 == "" {
		return nil
	}
	if key, ok := m.sharedKeys[pubKeyB64]; ok {
		return key
	}
	pubKey, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return nil
	}
	secret, err := crypto.SharedSecret(m.keyPair.PrivateKey, pubKey)
	if err != nil {
		return nil
	}
	key := crypto.DeriveKey(secret)
	m.sharedKeys[pubKeyB64] = key
	return key
}

func (m *Model) ensureChatInList(chatID, displayName string, online bool) {
	for _, c := range m.chatList {
		if c.ID == chatID {
			return
		}
	}
	item := ChatItem{
		ID:       chatID,
		Name:     displayName,
		IsGlobal: false,
		Online:   online,
	}
	m.chatList = append(m.chatList, item)
}

func (m *Model) handleSend() (tea.Model, tea.Cmd) {
	text := m.input.Value()
	text = strings.TrimSpace(text)
	if text == "" {
		return m, nil
	}
	m.input.SetValue("")

	if strings.HasPrefix(text, "/") {
		return m, m.handleCommand(text)
	}

	if !m.connected {
		m.addErrorMsg("Cannot send: not connected to relay")
		return m, nil
	}

	now := time.Now().Format("15:04:05")

	if m.activeChat == "global" {
		if err := m.client.SendGlobal(text); err != nil {
			m.addErrorMsg(fmt.Sprintf("Send error: %v", err))
			return m, nil
		}
		m.addMessage(m.username, text, now, true)
	} else {
		pubKey, ok := m.loadPubKey(m.activeChat)
		if !ok {
			m.addErrorMsg(fmt.Sprintf("No public key for %s", m.activeChat))
			return m, nil
		}
		sharedKey := m.getSharedKey(pubKey)
		if sharedKey == nil {
			m.addErrorMsg("Cannot derive encryption key")
			return m, nil
		}

		cipherText, nonce, err := crypto.EncryptMessage(sharedKey, text)
		if err != nil {
			m.addErrorMsg(fmt.Sprintf("Encryption error: %v", err))
			return m, nil
		}

		if err := m.client.SendDirect(m.activeChat, cipherText, nonce); err != nil {
			m.addErrorMsg(fmt.Sprintf("Send error: %v", err))
			return m, nil
		}

		m.db.SaveMessage(model.StoredMessage{
			Sender:      m.username,
			Recipient:   m.activeChat,
			Content:     cipherText,
			Nonce:       nonce,
			IsEncrypted: true,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		})

		m.addMessage(m.username, text, now, true)
	}

	return m, nil
}

func (m *Model) handleCommand(cmd string) tea.Cmd {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "/help":
		m.showHelp = !m.showHelp
		return nil

	case "/clear":
		m.messages = nil
		m.viewport.SetContent("")
		return nil

	case "/quit":
		return tea.Quit

	case "/panic":
		return m.panic()

	case "/sendfile":
		return m.handleSendFile(parts)

	case "/connect":
		if len(parts) >= 2 {
			addr := parts[1]
			m.addSystemMsg(fmt.Sprintf("Connecting to %s...", addr))
			m.client.Close()
			return m.reconnectWithAddr(addr)
		}
		m.addSystemMsg("Usage: /connect <relay_addr:port>")

	case "/nick":
		if len(parts) >= 2 {
			m.addSystemMsg(fmt.Sprintf("Username is set to: %s (restart to change)", m.username))
		}

	case "/whois":
		if len(parts) >= 2 {
			who := parts[1]
			if user, ok := m.users[who]; ok {
				pk := user.PubKey
				if len(pk) > 20 {
					pk = pk[:20] + "..."
				}
				m.addSystemMsg(fmt.Sprintf("User: %s | Online: %v | Key: %s",
					who, user.Online, pk))
			} else {
				m.addSystemMsg(fmt.Sprintf("User '%s' not found", who))
			}
		}

	default:
		m.addErrorMsg(fmt.Sprintf("Unknown command: %s (type /help)", parts[0]))
	}
	return nil
}

func (m *Model) handleSendFile(parts []string) tea.Cmd {
	if m.activeChat == "global" {
		m.addErrorMsg("Cannot send files in Global Chat")
		return nil
	}
	if len(parts) < 2 {
		m.addErrorMsg("Usage: /sendfile <local_path>")
		return nil
	}

	path := strings.Join(parts[1:], " ")

	data, err := os.ReadFile(path)
	if err != nil {
		m.addErrorMsg(fmt.Sprintf("Cannot read file: %v", err))
		return nil
	}

	filename := filepath.Base(path)
	filesize := int64(len(data))
	fileID := randomID()

	totalChunks := int(filesize / chunkSize)
	if filesize%chunkSize != 0 {
		totalChunks++
	}

	m.addSystemMsg(fmt.Sprintf("Sending %s (%s, %d chunks)...",
		filename, formatFileSize(filesize), totalChunks))

	if err := m.client.SendFileOffer(m.activeChat, filename, filesize, fileID); err != nil {
		m.addErrorMsg(fmt.Sprintf("Send error: %v", err))
		return nil
	}

	return func() tea.Msg {
		for i := 0; i < totalChunks; i++ {
			start := i * chunkSize
			end := start + chunkSize
			if end > len(data) {
				end = len(data)
			}
			chunkB64 := base64.StdEncoding.EncodeToString(data[start:end])
			m.client.SendFileChunk(m.activeChat, fileID, i+1, totalChunks, chunkB64)
		}
		return nil
	}
}

func (m *Model) handleNetMsg(msg model.Message) tea.Cmd {
	now := time.Now().Format("15:04:05")

	switch msg.Type {
	case model.MsgGlobal:
		if m.activeChat == "global" {
			m.addMessage(msg.Sender, msg.Content, now, false)
		} else {
			m.incrementUnread(msg.Sender)
		}

	case model.MsgDirect:
		if msg.Sender == m.username {
			return nil
		}
		sharedKey := m.getSharedKeyFromSender(msg.Sender)
		if sharedKey == nil {
			m.addErrorMsg(fmt.Sprintf("Cannot decrypt message from %s (no key)", msg.Sender))
			return nil
		}
		plaintext, err := crypto.DecryptMessage(sharedKey, msg.Content, msg.Nonce)
		if err != nil {
			m.addErrorMsg(fmt.Sprintf("Decryption error from %s: %v", msg.Sender, err))
			return nil
		}

		m.db.SaveMessage(model.StoredMessage{
			Sender:      msg.Sender,
			Recipient:   m.username,
			Content:     msg.Content,
			Nonce:       msg.Nonce,
			IsEncrypted: true,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		})
		m.db.UpdateContactLastSeen(msg.Sender)

		m.ensureChatInList(msg.Sender, msg.Sender, true)

		if m.activeChat == msg.Sender {
			m.addMessage(msg.Sender, plaintext, now, false)
		} else {
			m.incrementUnread(msg.Sender)
		}

		go m.client.SendReadReceipt(msg.Sender)

	case model.MsgPresence:
		switch msg.Status {
		case "online":
			if msg.Sender != "" {
				m.users[msg.Sender] = &model.User{
					Username: msg.Sender,
					PubKey:   msg.PubKey,
					Online:   true,
				}
				if msg.PubKey != "" {
					m.db.SaveContact(msg.Sender, msg.PubKey)
				}
				m.ensureChatInList(msg.Sender, msg.Sender, true)
				m.updateChatOnline(msg.Sender, true)
				m.addSystemMsg(fmt.Sprintf("✔ %s is online", msg.Sender))
			}
		case "offline":
			if msg.Sender != "" {
				if user, ok := m.users[msg.Sender]; ok {
					user.Online = false
				}
				m.updateChatOnline(msg.Sender, false)
				m.addSystemMsg(fmt.Sprintf("✖ %s went offline", msg.Sender))
			}
		case "userlist":
			var entries []struct {
				Username string `json:"username"`
				PubKey   string `json:"pubkey"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &entries); err == nil {
				for _, entry := range entries {
					if entry.Username != m.username {
						m.users[entry.Username] = &model.User{
							Username: entry.Username,
							PubKey:   entry.PubKey,
							Online:   true,
						}
						if entry.PubKey != "" {
							m.db.SaveContact(entry.Username, entry.PubKey)
						}
						m.ensureChatInList(entry.Username, entry.Username, true)
					}
				}
			}
		}

	case model.MsgTyping:
		if msg.Sender != "" {
			m.typing[msg.Sender] = time.Now()
		}

	case model.MsgRead:
		if m.activeChat == msg.Sender {
			m.addSystemMsg(fmt.Sprintf("✓ %s read your message", msg.Sender))
		}

	case model.MsgFileOffer:
		if msg.Sender == m.username {
			return nil
		}
		m.fileXfers[msg.FileID] = &FileTransfer{
			FileID:      msg.FileID,
			FileName:    msg.FileName,
			FileSize:    msg.FileSize,
			Sender:      msg.Sender,
			TotalChunks: 0,
			InProgress:  true,
			OutputPath:  filepath.Join(m.dataDir, "downloads", msg.FileName),
		}
		m.addFileMsg(msg.Sender, msg.FileName, msg.FileSize)
		m.addSystemMsg(fmt.Sprintf("Receiving file: %s", msg.FileName))

	case model.MsgFileChunk:
		if xfer, ok := m.fileXfers[msg.FileID]; ok && xfer.InProgress {
			chunk, err := base64.StdEncoding.DecodeString(msg.ChunkData)
			if err != nil {
				return nil
			}
			if xfer.Chunks == nil {
				xfer.Chunks = make([][]byte, msg.ChunkTotal)
				xfer.TotalChunks = msg.ChunkTotal
			}
			idx := msg.ChunkSeq - 1
			if idx >= 0 && idx < msg.ChunkTotal {
				xfer.Chunks[idx] = chunk
				xfer.Received++
			}
			m.addFileProgress(msg.FileID, xfer.Received, xfer.TotalChunks)

			if xfer.Received >= xfer.TotalChunks {
				m.assembleFile(xfer)
			}
		}

	case model.MsgError:
		m.addErrorMsg(msg.Content)
	}

	return nil
}

func (m *Model) assembleFile(xfer *FileTransfer) {
	os.MkdirAll(filepath.Dir(xfer.OutputPath), 0700)
	out, err := os.Create(xfer.OutputPath)
	if err != nil {
		return
	}
	defer out.Close()

	for _, chunk := range xfer.Chunks {
		if chunk != nil {
			out.Write(chunk)
		}
	}
	xfer.InProgress = false
}

func (m *Model) getSharedKeyFromSender(sender string) []byte {
	if user, ok := m.users[sender]; ok && user.PubKey != "" {
		return m.getSharedKey(user.PubKey)
	}
	pubKey, err := m.db.GetPubKey(sender)
	if err != nil {
		return nil
	}
	return m.getSharedKey(pubKey)
}

func (m *Model) incrementUnread(chatID string) {
	for i := range m.chatList {
		if m.chatList[i].ID == chatID {
			m.chatList[i].Unread++
			break
		}
	}
}

func (m *Model) updateChatOnline(chatID string, online bool) {
	for i := range m.chatList {
		if m.chatList[i].ID == chatID {
			m.chatList[i].Online = online
			break
		}
	}
}

func (m *Model) panic() tea.Cmd {
	m.addSystemMsg("!!! PANIC: Wiping all local data !!!")

	if err := m.db.Wipe(); err != nil {
		m.addErrorMsg(fmt.Sprintf("DB wipe error: %v", err))
	}

	if m.keyPath != "" {
		crypto.WipeKeyFile(m.keyPath)
	}

	m.client.Close()

	m.messages = nil
	msg := PanicStyle.Render("!!! ALL LOCAL DATA WIPED - APPLICATION CLOSING !!!")
	m.messages = append(m.messages, msg)
	m.viewport.SetContent(strings.Join(m.messages, "\n"))

	return func() tea.Msg {
		return panicDone{}
	}
}

func (m *Model) reconnect() tea.Cmd {
	m.addSystemMsg("Attempting to reconnect...")
	addr := m.client.Addr()
	return func() tea.Msg {
		time.Sleep(3 * time.Second)
		newClient := network.NewClient(addr, m.username, m.keyPair.PublicKey)
		if err := newClient.Connect(); err != nil {
			return reconnectFailed{}
		}
		return reconnectSuccess{client: newClient, msgCh: newClient.Messages()}
	}
}

func (m *Model) reconnectWithAddr(addr string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1 * time.Second)
		newClient := network.NewClient(addr, m.username, m.keyPair.PublicKey)
		if err := newClient.Connect(); err != nil {
			return reconnectFailed{}
		}
		return reconnectSuccess{client: newClient, msgCh: newClient.Messages()}
	}
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	sidebar := m.renderSidebar()
	mainArea := m.renderMain()

	line := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainArea)
	return AppStyle.Render(line)
}

func (m Model) renderSidebar() string {
	var b strings.Builder

	b.WriteString(SidebarTitleStyle.Render("TeChat"))
	b.WriteString("\n")

	sep := SeparatorStyle.Render(strings.Repeat("─", sidebarWidth-2))
	b.WriteString(sep)
	b.WriteString("\n")

	for i, chat := range m.chatList {
		var line string
		cursor := "  "
		if i == m.selChatIdx && !m.focusInput {
			cursor = "▸ "
		}

		name := chat.Name
		if len(name) > 16 {
			name = name[:16]
		}

		status := OfflineIndicator.Render("○")
		if chat.Online {
			status = OnlineIndicator.Render("●")
		}

		unread := ""
		if chat.Unread > 0 {
			unread = UnreadStyle.Render(fmt.Sprintf(" %d", chat.Unread))
		}

		chatStr := fmt.Sprintf("%s%s %s%s", cursor, status, name, unread)
		if i == m.selChatIdx && !m.focusInput {
			line = ActiveChatStyle.Render(fmt.Sprintf("%-*s", sidebarWidth-2, chatStr))
		} else {
			line = InactiveChatStyle.Render(fmt.Sprintf("%-*s", sidebarWidth-2, chatStr))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString(sep)
	b.WriteString("\n")

	// Users section header
	b.WriteString(InactiveChatStyle.Render(fmt.Sprintf("%-*s", sidebarWidth-2, "── Online ──")))
	b.WriteString("\n")

	onlineCount := 0
	for _, chat := range m.chatList {
		if chat.Online && !chat.IsGlobal {
			onlineCount++
		}
	}
	statusLine := fmt.Sprintf(" %d online", onlineCount)
	b.WriteString(InactiveChatStyle.Render(fmt.Sprintf("%-*s", sidebarWidth-2, statusLine)))

	return SidebarStyle.Render(b.String())
}

func (m Model) renderMain() string {
	var b strings.Builder

	chatHeader := m.renderChatHeader()
	b.WriteString(chatHeader)
	b.WriteString("\n")

	vpContent := m.viewport.View()
	b.WriteString(vpContent)
	b.WriteString("\n")

	fillerHeight := m.viewport.Height - strings.Count(vpContent, "\n") - 1
	if fillerHeight > 0 {
		b.WriteString(strings.Repeat("\n", fillerHeight))
	}

	b.WriteString(m.renderInputArea())

	b.WriteString(m.renderStatusBar())

	return ChatAreaStyle.Render(b.String())
}

func (m Model) renderChatHeader() string {
	title := m.activeChat
	if title == "global" {
		title = "🌐 Global Chat"
	} else {
		online := false
		for _, chat := range m.chatList {
			if chat.ID == title {
				online = chat.Online
				break
			}
		}
		status := OfflineIndicator.Render(" ○ offline")
		if online {
			status = OnlineIndicator.Render(" ● online")
		}
		if title != "" {
			title = fmt.Sprintf("✉ %s%s", title, status)
		}
	}

	typingText := m.renderTypingIndicator()
	header := fmt.Sprintf(" %s  %s", title, typingText)
	width := m.width - sidebarWidth - 4
	if width < 10 {
		width = 10
	}
	return TitleStyle.Render(fmt.Sprintf("%-*s", width, header))
}

func (m Model) renderTypingIndicator() string {
	now := time.Now()
	var typers []string
	for user, t := range m.typing {
		if now.Sub(t) < 3*time.Second {
			typers = append(typers, user)
		}
	}
	if len(typers) > 0 {
		return TypingStyle.Render(fmt.Sprintf("(%s typing...) ", strings.Join(typers, ", ")))
	}
	return ""
}

func (m Model) renderInputArea() string {
	var b strings.Builder

	width := m.width - sidebarWidth - 4
	if width < 10 {
		width = 10
	}

	inputView := m.input.View()
	b.WriteString(InputAreaStyle.Render(fmt.Sprintf("%-*s", width, inputView)))

	return b.String()
}

func (m Model) renderStatusBar() string {
	var b strings.Builder

	connStatus := OfflineIndicator.Render("● DISCONNECTED")
	if m.connected {
		connStatus = OnlineIndicator.Render("● CONNECTED")
	}

	helpHint := " [?] help"
	if m.showHelp {
		helpHint = m.renderHelp()
	}

	status := fmt.Sprintf(" %s  %s  %s", connStatus, m.username, helpHint)
	width := m.width - sidebarWidth - 4
	if width < 10 {
		width = 10
	}
	b.WriteString(StatusBarStyle.Render(fmt.Sprintf("%-*s", width, status)))

	return b.String()
}

func (m Model) renderHelp() string {
	return `[Tab:nav] [Enter:send] [Alt+Enter:newline] [/cmd] [Ctrl+Esc:panic]`
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

func randomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 16)
	for i := range id {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		id[i] = charset[n.Int64()]
	}
	return string(id)
}
