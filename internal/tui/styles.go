package tui

import "github.com/charmbracelet/lipgloss"

var (
	Green      = lipgloss.Color("#00ff00")
	DarkGreen  = lipgloss.Color("#00aa00")
	Darker     = lipgloss.Color("#008800")
	DimGreen   = lipgloss.Color("#004400")
	Red        = lipgloss.Color("#ff0000")
	BrightRed  = lipgloss.Color("#ff4444")
	Black      = lipgloss.Color("#000000")
	Gray       = lipgloss.Color("#555555")
	LightGray  = lipgloss.Color("#aaaaaa")
	Yellow     = lipgloss.Color("#ffff00")
	Cyan       = lipgloss.Color("#00ffff")

	AppStyle = lipgloss.NewStyle().Background(Black)

	SidebarStyle = lipgloss.NewStyle().
		Width(22).
		Border(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		Padding(0, 1).
		Background(Black)

	SidebarTitleStyle = lipgloss.NewStyle().
		Foreground(Green).
		Bold(true).
		Background(Black).
		Padding(0, 1)

	ActiveChatStyle = lipgloss.NewStyle().
		Foreground(Black).
		Background(Green).
		Bold(true).
		Padding(0, 1)

	InactiveChatStyle = lipgloss.NewStyle().
		Foreground(DarkGreen).
		Background(Black).
		Padding(0, 1)

	OnlineIndicator = lipgloss.NewStyle().
			Foreground(Green)

	OfflineIndicator = lipgloss.NewStyle().
			Foreground(Gray)

	ChatAreaStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Background(Black)

	MessageTimeStyle = lipgloss.NewStyle().
			Foreground(Darker).
			Background(Black)

	SenderStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
			Background(Black)

	SelfSenderStyle = lipgloss.NewStyle().
			Foreground(DarkGreen).
			Background(Black)

	MessageBodyStyle = lipgloss.NewStyle().
			Foreground(DarkGreen).
			Background(Black)

	SystemMsgStyle = lipgloss.NewStyle().
			Foreground(Darker).
			Italic(true).
			Background(Black)

	ErrorMsgStyle = lipgloss.NewStyle().
			Foreground(Red).
			Background(Black)

	TypingStyle = lipgloss.NewStyle().
			Foreground(Darker).
			Italic(true).
			Background(Black)

	InputAreaStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			Padding(0, 1).
			Background(Black)

	StatusBarStyle = lipgloss.NewStyle().
			Height(1).
			Padding(0, 1).
			Background(Black).
			Foreground(Darker)

	TitleStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
			Background(Black).
			Align(lipgloss.Center).
			Width(22)

	UnreadStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true).
			Background(Black)

	CodeBlockStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Background(Black).
			Padding(0, 2)

	BoldStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true).
			Background(Black)

	ItalicStyle = lipgloss.NewStyle().
			Foreground(DarkGreen).
			Italic(true).
			Background(Black)

	FileMsgStyle = lipgloss.NewStyle().
			Foreground(Yellow).
			Background(Black)

	FileProgressStyle = lipgloss.NewStyle().
				Foreground(Cyan).
				Background(Black)

	SeparatorStyle = lipgloss.NewStyle().
			Foreground(Darker).
			Background(Black)

	PanicStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true).
			Background(Black)
)
