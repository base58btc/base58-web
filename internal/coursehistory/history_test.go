package coursehistory

import "testing"

func TestUndoRetainsFailedCommandsAndBranches(t *testing.T) {
	s := New()
	for _, e := range []Event{
		{ID: "a", Type: "command", Command: &Command{Program: "bitcoin-cli", Args: []string{"createwallet", "course"}}},
		{ID: "ar", Type: "result", CommandID: "a", Result: &Result{ExitCode: 0}},
		{ID: "b", Type: "command", Parent: "a", Command: &Command{Program: "bash", Args: []string{"-c", "touch partial; exit 1"}}},
		{ID: "br", Type: "result", CommandID: "b", Result: &Result{ExitCode: 1}},
		{ID: "undo", Type: "checkout", Head: "a"},
		{ID: "c", Type: "command", Parent: "a", Command: &Command{Program: "pwd"}},
		{ID: "cr", Type: "result", CommandID: "c", Result: &Result{}},
	} {
		if err := s.Apply(e); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
	}
	if s.Head != "c" || len(s.Commands) != 3 || s.Commands["b"].Result.ExitCode != 1 {
		t.Fatal("lost branch or failed command")
	}
	if s.Apply(Event{ID: "bad", Type: "checkout", Head: "b"}) == nil {
		t.Fatal("allowed unrelated branch checkout")
	}
}

func TestInterruptedCommandDoesNotAdvanceHead(t *testing.T) {
	s := New()
	if err := s.Apply(Event{ID: "a", Type: "command", Command: &Command{Program: "sleep", Args: []string{"30"}}}); err != nil {
		t.Fatal(err)
	}
	if s.Apply(Event{ID: "b", Type: "command", Command: &Command{Program: "pwd"}}) == nil {
		t.Fatal("allowed concurrent command")
	}
	if s.Apply(Event{ID: "r", Type: "result", CommandID: "other", Result: &Result{}}) == nil {
		t.Fatal("allowed mismatched result")
	}
	if err := s.Apply(Event{ID: "i", Type: "interrupted", CommandID: "a"}); err != nil {
		t.Fatal(err)
	}
	if s.Head != "" || s.Pending != "" || !s.Commands["a"].Interrupted {
		t.Fatal("incorrect recovery")
	}
}

func TestCommandTimeoutBounds(t *testing.T) {
	for _, timeout := range []int{0, 30000, 120000, -1, 120001} {
		state := New()
		err := state.Apply(Event{ID: "command", Type: "command", Command: &Command{Program: "python3", TimeoutMS: timeout}})
		if (err != nil) != (timeout < 0 || timeout > 120000) {
			t.Fatalf("timeout %d: %v", timeout, err)
		}
	}
}

func TestResetArchivesCommandsAndStartsANewRoot(t *testing.T) {
	s := New()
	apply := func(e Event) {
		t.Helper()
		if err := s.Apply(e); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
	}
	apply(Event{ID: "old", Type: "command", Command: &Command{Program: "pwd"}})
	if s.Apply(Event{ID: "pending-reset", Type: "reset"}) == nil {
		t.Fatal("reset discarded an unfinished command")
	}
	apply(Event{ID: "result", Type: "result", CommandID: "old", Result: &Result{Stdout: "preserved"}})
	if s.Apply(Event{ID: "bad-reset", Type: "reset", Head: "old"}) == nil {
		t.Fatal("allowed reset with a cursor")
	}
	apply(Event{ID: "reset", Type: "reset"})
	if s.Head != "" || s.Commands["old"].Result.Stdout != "preserved" {
		t.Fatal("reset lost history or retained replay cursor")
	}
	if s.Apply(Event{ID: "restore-old", Type: "checkout", Head: "old"}) == nil {
		t.Fatal("allowed checkout across reset")
	}
	if s.Apply(Event{ID: "extend-old", Type: "command", Parent: "old", Command: &Command{Program: "pwd"}}) == nil {
		t.Fatal("allowed command on archived workspace")
	}
	apply(Event{ID: "new", Type: "command", Command: &Command{Program: "pwd"}})
	apply(Event{ID: "new-result", Type: "result", CommandID: "new", Result: &Result{}})
	apply(Event{ID: "undo", Type: "checkout"})
	if s.Head != "" || len(s.Commands) != 2 {
		t.Fatal("undo crossed reset or lost archive")
	}
}
