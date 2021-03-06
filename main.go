package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/logrusorgru/aurora"
	clr "github.com/logrusorgru/aurora/v3"
	"github.com/osuushi/merry-go-round/pipemap"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--watcher" {
		watch()
	} else {
		tmux()
	}
}

func getMTime() (time.Time, error) {
	stat, err := os.Stat("./main.go")
	if err != nil {
		return time.Time{}, err
	}
	return stat.ModTime(), nil
}

func watch() {
	fmt.Println(os.Getenv("TERM"))

	session := os.Getenv("MGR_SESSION")
	if session == "" {
		log.Fatal("Cannot run --watcher outside of tmux session")
	}

	// If we get a sigint, kill the entire tmux session; editor and output should run as a single process
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		for {
			<-sigs
			fmt.Println(clr.Red("To stop merry-go-round, quit the micro session"))
		}
	}()

	var lastMTime time.Time
	var err error
	lastMTime, err = getMTime()

	pauseAfterMtimeError := func(err error) {
		if err != nil {
			fmt.Println("Error getting mtime for main.go:", err, "... We'll try to recover in five seconds.")
			time.Sleep(5 * time.Second)
		}
	}

	pauseAfterMtimeError(err)

	fmt.Println(clr.Cyan("Watching for changes..."))
	for {
		newMTime, err := getMTime()
		pauseAfterMtimeError(err)
		if newMTime.After(lastMTime) {
			fmt.Println(clr.Yellow("Running..."))
			cmd := exec.Command("bash", "-c", `
				clear
				go mod tidy
				go run .
			`)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout

			stderrPipe, err := cmd.StderrPipe()
			if err == nil {
				stderrCh := pipemap.Strings(stderrPipe, os.Stderr, func(s string) string { return aurora.Red(s).String() })
				err := cmd.Start()
				if err != nil {
					fmt.Println(clr.Red(fmt.Sprintf("Error starting: %v", err)))
				}

				<-stderrCh
				err = cmd.Wait()
				if err != nil {
					fmt.Println(clr.Red(fmt.Sprintf("Error starting: %v", err)))
				}
			} else {
				fmt.Println(clr.Red(fmt.Sprintf("Error piping stderr: %v", err)))
			}

			// Note that we don't use newMTime here because some time has passed and
			// we want to skip over any modifications during that time rather than
			// rerunning.
			lastMTime, err = getMTime()
			pauseAfterMtimeError(err)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func tmux() {
	sessionId := generateSessionId()
	self, err := os.Executable()
	if err != nil {
		log.Fatal("Getting absolute path to merry-go-round: ", err)
	}

	scriptTemplateString := strings.Join([]string{
		`set -ex`,
		`cd {{.TmpDir}}`,
		`echo 'package main' > main.go`,
		`echo >> main.go`,
		`echo 'func main() {' >> main.go`,
		`echo "	" >> main.go`,
		`echo '}' >> main.go`,
		`go mod init {{.SessionId}}`,

		// Create the session for the editor, and set the session to be killed once
		// the editor closes
		`tmux new-session -d -s {{.SessionId}}`,
		`tmux set mouse on`,

		// All of the internet wisdom insists that setting a terminal other than
		// screen* or tmux* will break everything, but this is the only value I've
		// found where ctrl-arrow and shift-home/end work properly in micro, and
		// which has a terminal definition. "screen+fkeys" also works, at least in
		// iTerm, but is more obscure.
		`tmux set default-terminal 'xterm-256color'`,
		`tmux split-window "micro main.go +4:2; tmux kill-session -t {{.SessionId}}"`,
		`tmux kill-pane -t 0`,

		// Split off a the polling instance of merry-go-round
		`tmux split-window -t {{.SessionId}} -v '{{.Self}} --watcher'`,
		`tmux select-pane -t 0`,
		`tmux attach-session -t {{.SessionId}}`,
	}, "\n")

	tpl := template.Must(template.New("tmux-setup").Parse(scriptTemplateString))
	var buf bytes.Buffer
	tpl.Execute(&buf, map[string]interface{}{
		"TmpDir":    makeTempDir(),
		"SessionId": sessionId,
		"Self":      self,
	})

	script := buf.String()
	env := os.Environ()
	env = append(env, "MGR_SESSION="+sessionId)
	syscall.Exec("/bin/bash", []string{"-i", "-c", script}, env)
}

func makeTempDir() string {
	dir, err := os.MkdirTemp(os.TempDir(), "mgr-")
	if err != nil {
		log.Fatal("Error trying to create temporary directory:", err)
	}
	return dir
}

func generateSessionId() string {
	bytes := make([]byte, 6)

	if _, err := rand.Read(bytes); err != nil {
		log.Fatal("Generate session id:", err)
	}
	s := base64.RawURLEncoding.EncodeToString(bytes)
	// We don't need a perfect distribution, so we're basically generating a
	// slightly skewed base62 instead. This is purely aesthetic.
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return "merry-go-round-" + s
}
