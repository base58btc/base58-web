// Package coursehistory validates the append-only CLI journal for an attempt.
package coursehistory

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type Command struct {
	Program    string   `json:"program"`
	Args       []string `json:"args"`
	Source     string   `json:"source,omitempty"`
	LessonPath string   `json:"lessonPath,omitempty"`
	BlockID    string   `json:"blockId,omitempty"`
}
type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	TimedOut bool   `json:"timedOut"`
}
type Event struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Time      string   `json:"time"`
	Parent    string   `json:"parent,omitempty"`
	Command   *Command `json:"command,omitempty"`
	CommandID string   `json:"commandId,omitempty"`
	Result    *Result  `json:"result,omitempty"`
	Head      string   `json:"head,omitempty"`
}
type Entry struct {
	Event       Event
	Result      *Result
	Interrupted bool
}
type State struct {
	Head     string
	Pending  string
	Commands map[string]*Entry
	IDs      map[string]bool
}

var ErrInvalid = errors.New("invalid command journal transition")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func New() *State { return &State{Commands: map[string]*Entry{}, IDs: map[string]bool{}} }

func (s *State) Apply(e Event) error {
	if !idPattern.MatchString(e.ID) || s.IDs[e.ID] || len(e.Time) > 64 {
		return ErrInvalid
	}
	switch e.Type {
	case "command":
		if s.Pending != "" || e.Parent != s.Head || e.Command == nil || e.Result != nil {
			return ErrInvalid
		}
		c := e.Command
		data, _ := json.Marshal(c)
		if c.Program == "" || len(data) > 65536 || len(c.Args) > 1024 || strings.ContainsRune(c.Program, 0) {
			return ErrInvalid
		}
		for _, arg := range c.Args {
			if strings.ContainsRune(arg, 0) {
				return ErrInvalid
			}
		}
		s.Commands[e.ID] = &Entry{Event: e}
		s.Pending = e.ID
	case "result", "interrupted":
		if s.Pending == "" || e.CommandID != s.Pending {
			return ErrInvalid
		}
		entry := s.Commands[e.CommandID]
		if e.Type == "result" {
			if e.Result == nil || len(e.Result.Stdout) > 1<<20 || len(e.Result.Stderr) > 1<<20 {
				return ErrInvalid
			}
			entry.Result = e.Result
			s.Head = e.CommandID
		} else {
			entry.Interrupted = true
		}
		s.Pending = ""
	case "checkout":
		if s.Pending != "" {
			return ErrInvalid
		}
		// An undo may only select an ancestor. Old branches remain in the log.
		cursor := s.Head
		for cursor != "" && cursor != e.Head {
			cursor = s.Commands[cursor].Event.Parent
		}
		if cursor != e.Head {
			return ErrInvalid
		}
		s.Head = e.Head
	default:
		return ErrInvalid
	}
	s.IDs[e.ID] = true
	return nil
}
