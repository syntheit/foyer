// foyer-api is a standalone CLI client that authenticates to Foyer's API using your SSH key.
// Usage: foyer-api [--host URL] [--key PATH] METHOD /path
//
// Examples:
//   foyer-api --host https://harbor.matv.io GET /api/health
//   foyer-api --host https://harbor.matv.io GET /api/health/cpu
//   foyer-api GET /api/jellyfin/streams
package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dmiller/foyer/internal/auth"
	"golang.org/x/crypto/ssh"
)

func main() {
	host := flag.String("host", "http://localhost:8420", "Foyer server URL")
	keyPath := flag.String("key", "", "path to SSH private key (default: ~/.ssh/id_ed25519)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: foyer-api [--host URL] [--key PATH] [METHOD] /path\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  foyer-api --host https://harbor.matv.io /api/health\n")
		fmt.Fprintf(os.Stderr, "  foyer-api --host https://harbor.matv.io GET /api/health/cpu\n")
		os.Exit(1)
	}

	// Parse method and path
	method := "GET"
	apiPath := args[0]
	if len(args) >= 2 {
		method = strings.ToUpper(args[0])
		apiPath = args[1]
	}

	// Resolve key path
	if *keyPath == "" {
		home, _ := os.UserHomeDir()
		// Try ed25519 first, then rsa
		candidates := []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "mainkey"),
			filepath.Join(home, ".ssh", "id_rsa"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				*keyPath = c
				break
			}
		}
		if *keyPath == "" {
			fmt.Fprintf(os.Stderr, "error: no SSH private key found in ~/.ssh/\n")
			os.Exit(1)
		}
	}

	// Read and parse private key
	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read key %s: %v\n", *keyPath, err)
		os.Exit(1)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot parse key %s: %v\n", *keyPath, err)
		os.Exit(1)
	}

	// Build the signed message
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := auth.BuildSignedMessage(timestamp, method, apiPath)

	// Sign
	sig, err := signer.Sign(rand.Reader, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: signing failed: %v\n", err)
		os.Exit(1)
	}

	sigB64 := base64.StdEncoding.EncodeToString(ssh.Marshal(sig))

	// Make the request
	url := strings.TrimRight(*host, "/") + apiPath
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("X-SSH-Timestamp", timestamp)
	req.Header.Set("X-SSH-Signature", sigB64)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	io.Copy(os.Stdout, resp.Body)

	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}
