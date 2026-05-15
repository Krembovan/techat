package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Krembovan/techat/internal/crypto"
	"github.com/Krembovan/techat/internal/db"
	"github.com/Krembovan/techat/internal/network"
	"github.com/Krembovan/techat/internal/tui"
)

func main() {
	var relayAddr string
	var username string
	var dataDir string

	flag.StringVar(&relayAddr, "relay", "127.0.0.1:7777", "Relay server address")
	flag.StringVar(&username, "username", "", "Display name")
	flag.StringVar(&dataDir, "data", "", "Data directory (default: ~/.techat)")
	flag.Parse()

	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			username = "anonymous"
		} else {
			username = currentUser.Username
		}
	}

	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot find home directory: %v\n", err)
			os.Exit(1)
		}
		dataDir = filepath.Join(home, ".techat")
	}

	os.MkdirAll(dataDir, 0700)

	keyPath := filepath.Join(dataDir, "keys.json")
	keyPair, err := crypto.LoadOrCreateKeys(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load/create keys: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(dataDir, "messages.db")
	database, err := db.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	contacts, _ := database.GetContacts()
	for _, c := range contacts {
		database.SaveContact(c.Username, c.PubKey)
	}

	client := network.NewClient(relayAddr, username, keyPair.PublicKey)

	if err := client.Connect(); err != nil {
		log.Printf("Warning: Could not connect to relay at %s: %v", relayAddr, err)
		log.Printf("Running in offline mode. Use /connect <addr> to connect.")
	}

	p := tea.NewProgram(
		tui.New(username, client, database, keyPair, keyPath, dataDir),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client.Close()
}
