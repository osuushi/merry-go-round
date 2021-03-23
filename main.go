package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"
)

func main() {
	fmt.Println(os.Args[0])
	if true {
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "watcher" {
		watch()
	} else {
		tmux()
	}
}

func watch() {
	time.Sleep(30 * time.Second)
}

func tmux() {
	sessionId := generateSessionId()
	scriptTemplateString := `` +
		// Create the session for the editor, and set the session to be killed once the editor closes
		`tmux new-session -s {{.SessionId}} "'{{.Editor}}' && tmux kill-session -t {{.SessionId}}"` +
		`tmux split-window -v '{{.Self}} watch'`

	tpl := template.Must(template.New("tmux-setup").Parse(scriptTemplateString))
	var buf bytes.Buffer
	tpl.Execute(&buf, map[string]interface{}{
		"SessionId": sessionId,
		"Editor":    getEditor(),
		"Self":      os.Args[0],
	})
	script := buf.String()
	fmt.Println(script)
}

func getEditor() string {
	envVars := []string{"MGR_EDITOR", "VISUAL", "EDITOR"}
	for _, v := range envVars {
		val := os.Getenv(v)
		if val != "" {
			return val
		}
	}
	// Fall back to nano
	return "nano"
}

func generateSessionId() string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		panic(err.Error())
	}
	s := base64.RawURLEncoding.EncodeToString(bytes)
	// We don't need a perfect distribution, so we're basically generating a
	// slightly skewed base62 instead. This is purely aesthetic.
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return "merry-go-round-" + s
}
